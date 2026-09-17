package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// gitDisplayPath walks up from the file to find a .git dir, then returns
// "project-name/relative/path". Falls back to the original path.
func gitDisplayPath(absPath string) string {
	if absPath == "" || !filepath.IsAbs(absPath) {
		return absPath
	}
	dir := filepath.Dir(absPath)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			rel, err := filepath.Rel(dir, absPath)
			if err != nil {
				break
			}
			return filepath.Base(dir) + "/" + rel
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return absPath
}

type Annotation struct {
	Text    string `json:"text"`
	Comment string `json:"comment"`
}

type Event struct {
	Type        string       `json:"type"`
	File        string       `json:"file,omitempty"`
	Content     string       `json:"content,omitempty"`
	Lang        string       `json:"lang,omitempty"`
	Message     string       `json:"message,omitempty"`
	Description string       `json:"description,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

type Response struct {
	Decision string `json:"decision"` // "accept" or "reject"
	Message  string `json:"message,omitempty"`
}

const maxHistory = 50

type Server struct {
	bind         string
	port         int
	mu           sync.Mutex
	clients      map[chan string]struct{}
	history      []*Event // show/diff events only
	activePrompt string
	responseCh   chan Response
	lastResponse *Response
}

func New(bind string, port int) *Server {
	return &Server{
		bind:    bind,
		port:    port,
		clients: make(map[chan string]struct{}),
	}
}

func (s *Server) pushHistory(e *Event) {
	if len(s.history) >= maxHistory {
		s.history = s.history[1:]
	}
	s.history = append(s.history, e)
}

func (s *Server) broadcast(e *Event) {
	data, _ := json.Marshal(e)
	msg := "data: " + string(data) + "\n\n"
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (s *Server) subscribe() chan string {
	ch := make(chan string, 8)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan string) {
	s.mu.Lock()
	delete(s.clients, ch)
	s.mu.Unlock()
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/api/show", s.handleShow)
	mux.HandleFunc("/api/diff", s.handleDiff)
	mux.HandleFunc("/api/clear", s.handleClear)
	mux.HandleFunc("/api/prompt", s.handlePrompt)
	mux.HandleFunc("/api/respond", s.handleRespond)
	mux.HandleFunc("/api/wait", s.handleWait)

	addr := fmt.Sprintf("%s:%d", s.bind, s.port)
	log.Printf("code-view listening on http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := s.subscribe()
	defer s.unsubscribe(ch)

	s.mu.Lock()
	history := make([]*Event, len(s.history))
	copy(history, s.history)
	activePrompt := s.activePrompt
	s.mu.Unlock()

	// Send full history as one event so browser can build its tab bar
	if len(history) > 0 {
		type historyMsg struct {
			Type   string   `json:"type"`
			Events []*Event `json:"events"`
		}
		data, _ := json.Marshal(historyMsg{Type: "history", Events: history})
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	if activePrompt != "" {
		data, _ := json.Marshal(&Event{Type: "prompt", Message: activePrompt})
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var e Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e.Type = "show"
	e.File = gitDisplayPath(e.File)
	s.mu.Lock()
	s.pushHistory(&e)
	s.mu.Unlock()
	s.broadcast(&e)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file := r.URL.Query().Get("file")
	e := &Event{Type: "diff", File: gitDisplayPath(file), Content: string(body)}
	s.mu.Lock()
	s.pushHistory(e)
	s.mu.Unlock()
	s.broadcast(e)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	e := &Event{Type: "clear"}
	s.broadcast(e)
	s.mu.Lock()
	s.history = nil
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ch := make(chan Response, 1)
	s.mu.Lock()
	s.responseCh = ch
	s.lastResponse = nil
	s.activePrompt = req.Message
	s.mu.Unlock()

	s.broadcast(&Event{Type: "prompt", Message: req.Message})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRespond(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var resp Response
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	ch := s.responseCh
	s.responseCh = nil
	s.mu.Unlock()

	s.mu.Lock()
	s.lastResponse = &resp
	s.activePrompt = ""
	s.mu.Unlock()

	if ch != nil {
		select {
		case ch <- resp:
		default:
		}
	}

	s.broadcast(&Event{Type: "prompt_clear"})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ch := s.responseCh
	last := s.lastResponse
	s.mu.Unlock()

	// Already responded before we started waiting
	if last != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(last)
		return
	}

	if ch == nil {
		http.Error(w, "no active prompt", http.StatusConflict)
		return
	}

	select {
	case resp := <-ch:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	case <-time.After(5 * time.Minute):
		http.Error(w, "timeout", http.StatusRequestTimeout)
	case <-r.Context().Done():
	}
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, uiHTML)
}

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>code-view</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css">
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/diff2html/3.4.47/bundles/css/diff2html.min.css">
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #080b10; color: #c9d1d9; font-family: monospace; height: 100vh; display: flex; flex-direction: column; overflow: hidden; }

  #header { background: #161b22; border-bottom: 1px solid #30363d; padding: 6px 16px; display: flex; align-items: center; gap: 12px; flex-shrink: 0; user-select: none; }
  #header h1 { font-size: 13px; color: #58a6ff; font-weight: 600; }
  #hint { font-size: 11px; color: #484f58; }
  #status { margin-left: auto; font-size: 11px; color: #3fb950; }
  #zoom-label { font-size: 11px; color: #484f58; min-width: 40px; text-align: right; }

  #viewport { flex: 1; overflow: hidden; position: relative; cursor: grab; }
  #viewport.dragging { cursor: grabbing; }
  #canvas { position: absolute; top: 0; left: 0; transform-origin: 0 0; display: flex; flex-flow: row wrap; align-items: flex-start; align-content: flex-start; gap: 20px; padding: 30px; }

  .card { width: max-content; min-width: 420px; flex-shrink: 0; background: #161b22; border: 1px solid #30363d; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 24px rgba(0,0,0,0.4); transition: border-color 0.15s; }
  .card.latest { border-color: #58a6ff; box-shadow: 0 0 0 1px #58a6ff44, 0 4px 24px rgba(0,0,0,0.4); }
  .card-header { background: #1c2128; padding: 8px 12px; border-bottom: 1px solid #30363d; }
  .card-header .title { display: flex; align-items: center; gap: 6px; }
  .card-header .badge { display: inline-block; background: #238636; color: #fff; font-size: 10px; padding: 1px 6px; border-radius: 3px; flex-shrink: 0; }
  .card-header .badge.diff { background: #9a3412; }
  .card-header .fname { font-size: 11px; color: #8b949e; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .card-header .desc { font-size: 11px; color: #484f58; margin-top: 3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-style: italic; }

  mark.annotation { background: #f0c27444; border-radius: 2px; cursor: pointer; outline: 1px solid transparent; transition: outline-color 0.2s, background 0.2s; }
  mark.annotation.active { outline-color: #f0c274; background: #f0c27466; }

  /* position: relative makes this the offsetParent for its descendants (mark,
     pre, code) -- layoutAnnotations()'s offsetTop walk assumes exactly that,
     stopping here in one hop. Without it, none of mark/code/pre/.card/.card-body
     are positioned, so offsetParent skips all of them straight to #canvas,
     and the walk keeps climbing past its intended stop, wildly overshooting. */
  .card-body { display: flex; flex-direction: row; position: relative; }
  .card-body pre { flex: none; }
  .ann-panel { position: relative; width: 240px; min-height: 40px; flex-shrink: 0; display: none; border-left: 1px solid #21262d; }
  .ann-panel.has-items { display: block; }
  .ann-item { position: absolute; display: flex; align-items: flex-start; gap: 6px; left: 0; right: 0; padding: 0 8px; }
  .ann-arrow { color: #f0c274; font-size: 13px; flex-shrink: 0; line-height: 1.45; margin-top: 1px; }
  .ann-comment { color: #f0c274; font-size: 11px; background: #1c212888; border: 1px solid #f0c27466; border-radius: 4px; padding: 3px 8px; line-height: 1.45; white-space: pre-wrap; word-break: break-word; font-style: italic; }


  #nav-hint { position: fixed; bottom: 12px; right: 16px; font-size: 11px; color: #30363d; pointer-events: none; transition: color 0.3s; }
  #nav-hint.has-annotations { color: #f0c274; }
  .card pre { margin: 0 !important; border-radius: 0 !important; }
  .card code.hljs { padding: 12px !important; font-size: 11px; line-height: 1.45; display: block; white-space: pre; }
  .card .diff-wrap { font-size: 10px; }
  .card .diff-wrap .d2h-file-wrapper { border: none; border-radius: 0; margin: 0; }

  #placeholder { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: #21262d; font-size: 20px; pointer-events: none; }

  #prompt-bar { display: none; background: #1c2128; border-top: 2px solid #58a6ff; padding: 12px 20px; flex-shrink: 0; gap: 12px; align-items: center; }
  #prompt-bar.active { display: flex; }
  #prompt-message { flex: 1; font-size: 13px; color: #e6edf3; }
  #prompt-bar button { padding: 7px 18px; border: none; border-radius: 6px; font-family: monospace; font-size: 12px; font-weight: 600; cursor: pointer; }
  #btn-accept { background: #238636; color: #fff; }
  #btn-reject { background: #b91c1c; color: #fff; }

  .d2h-code-linenumber { background: #161b22; border-color: #30363d; color: #8b949e; }
  .d2h-ins { background: #0d4a23 !important; }
  .d2h-del { background: #4a0d0d !important; }
  .d2h-file-header { background: #1c2128; border-color: #30363d; color: #8b949e; }
</style>
</head>
<body>
<div id="header">
  <h1>code-view</h1>
  <span id="hint">scroll to zoom · drag to pan</span>
  <span id="zoom-label">100%</span>
  <span id="status">connecting...</span>
</div>
<div id="viewport">
  <div id="placeholder">waiting for content...</div>
  <div id="canvas"></div>
</div>
<div id="prompt-bar">
  <span id="prompt-message"></span>
  <button id="btn-accept">Accept</button>
  <button id="btn-reject">Reject</button>
</div>
<div id="nav-hint">← → navigate cards</div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/languages/go.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/languages/rust.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/diff2html/3.4.47/bundles/js/diff2html-ui.min.js"></script>
<script>
const CARD_PAD = 30;

const viewport = document.getElementById('viewport');
const canvas = document.getElementById('canvas');
const placeholder = document.getElementById('placeholder');
const promptBar = document.getElementById('prompt-bar');
const promptMessage = document.getElementById('prompt-message');
const zoomLabel = document.getElementById('zoom-label');
const statusEl = document.getElementById('status');

let scale = 1, panX = 0, panY = 0;
let cards = [];
let currentCardIdx = -1;
let annotations = []; // global list of {mark, comment} in order added
let currentAnnIdx = -1;

function applyTransform() {
  canvas.style.transform = 'translate(' + panX + 'px,' + panY + 'px) scale(' + scale + ')';
  zoomLabel.textContent = Math.round(scale * 100) + '%';
}

// Pan
let dragging = false, dragStartX, dragStartY, panStartX, panStartY;
viewport.addEventListener('mousedown', e => {
  if (e.target.closest('button')) return;
  dragging = true; dragStartX = e.clientX; dragStartY = e.clientY;
  panStartX = panX; panStartY = panY;
  viewport.classList.add('dragging');
});
window.addEventListener('mousemove', e => {
  if (!dragging) return;
  panX = panStartX + (e.clientX - dragStartX);
  panY = panStartY + (e.clientY - dragStartY);
  applyTransform();
});
window.addEventListener('mouseup', () => { dragging = false; viewport.classList.remove('dragging'); });

// Zoom around mouse
viewport.addEventListener('wheel', e => {
  e.preventDefault();
  const rect = viewport.getBoundingClientRect();
  const mx = e.clientX - rect.left, my = e.clientY - rect.top;
  const delta = e.deltaY > 0 ? 0.9 : 1.1;
  const newScale = Math.min(3, Math.max(0.1, scale * delta));
  panX = mx - (mx - panX) * (newScale / scale);
  panY = my - (my - panY) * (newScale / scale);
  scale = newScale;
  applyTransform();
}, { passive: false });

function makeCard(e, isLatest) {
  const card = document.createElement('div');
  card.className = 'card' + (isLatest ? ' latest' : '');

  const hdr = document.createElement('div');
  hdr.className = 'card-header';
  const title = document.createElement('div');
  title.className = 'title';
  const badge = document.createElement('span');
  badge.className = 'badge' + (e.type === 'diff' ? ' diff' : '');
  badge.textContent = e.type;
  const fname = document.createElement('span');
  fname.className = 'fname';
  fname.textContent = e.file || '';
  title.appendChild(badge);
  title.appendChild(fname);
  hdr.appendChild(title);
  if (e.description) {
    const desc = document.createElement('div');
    desc.className = 'desc';
    desc.textContent = e.description;
    hdr.appendChild(desc);
  }
  card.appendChild(hdr);

  if (e.type === 'show') {
    const cardBody = document.createElement('div');
    cardBody.className = 'card-body';
    const pre = document.createElement('pre');
    const code = document.createElement('code');
    code.className = 'hljs' + (e.lang ? ' language-' + e.lang : '');
    code.textContent = e.content;
    pre.appendChild(code);
    cardBody.appendChild(pre);
    const annPanel = document.createElement('div');
    annPanel.className = 'ann-panel';
    cardBody.appendChild(annPanel);
    card.appendChild(cardBody);
    hljs.highlightElement(code);
    if (e.annotations && e.annotations.length) {
      e.annotations.forEach(ann => applyAnnotation(code, ann.text, ann.comment, annPanel));
      annPanel.classList.add('has-items');
      updateNavHint();
      requestAnimationFrame(() => layoutAnnotations(annPanel));
    }
  } else if (e.type === 'diff') {
    const wrap = document.createElement('div');
    wrap.className = 'diff-wrap';
    card.appendChild(wrap);
    const ui = new Diff2HtmlUI(wrap, e.content, {
      drawFileList: false, matching: 'lines', outputFormat: 'side-by-side',
      highlight: true, renderNothingWhenEmpty: false,
    });
    ui.draw(); ui.highlightCode();
  }

  return card;
}

function rebuildCanvas(events) {
  canvas.innerHTML = '';
  cards = [];
  events.forEach((e, i) => {
    const card = makeCard(e, i === events.length - 1);
    canvas.appendChild(card);
    cards.push(card);
  });
  placeholder.style.display = events.length ? 'none' : 'flex';
}

function addCard(e) {
  placeholder.style.display = 'none';
  // Remove latest highlight from previous
  cards.forEach(c => c.classList.remove('latest'));
  const card = makeCard(e, true);
  canvas.appendChild(card);
  cards.push(card);
  // Pan to show new card
  panToLatest();
}

// fitAll wraps cards into a grid instead of always laying every card out in
// one ever-widening row (a handful of cards used to produce a screenshot
// several times wider than tall). Rather than assume a fixed card shape or
// solve for it with a closed-form formula -- which picks bad column counts
// once a min-width floor and gaps matter, e.g. always preferring 1 column
// over a demonstrably-better-fitting 2 -- this tries a range of candidate
// wrap-widths and keeps whichever actually maximizes the resulting scale
// (equivalently, wastes the least viewport space), using real measured
// layout rather than averages.
function fitAll() {
  if (!cards.length) return;
  const vw = viewport.clientWidth;
  const vh = viewport.clientHeight;

  canvas.style.width = 'max-content';
  let maxCardW = 0;
  cards.forEach(c => { maxCardW = Math.max(maxCardW, c.offsetWidth); });
  const naturalWidth = canvas.scrollWidth; // every card in a single row

  function measureAt(w) {
    canvas.style.width = w + 'px';
    let maxRight = 0, maxBottom = 0;
    cards.forEach(c => {
      maxRight = Math.max(maxRight, c.offsetLeft + c.offsetWidth);
      maxBottom = Math.max(maxBottom, c.offsetTop + c.offsetHeight);
    });
    return { w: maxRight + CARD_PAD, h: maxBottom + CARD_PAD };
  }

  const SAMPLES = 30;
  const step = Math.max(10, Math.round((naturalWidth - maxCardW) / SAMPLES));
  let bestWidth = maxCardW;
  let best = measureAt(bestWidth);
  let bestFactor = Math.max(best.w / vw, best.h / vh);
  for (let w = maxCardW + step; w <= naturalWidth; w += step) {
    const box = measureAt(w);
    const factor = Math.max(box.w / vw, box.h / vh);
    if (factor < bestFactor) { bestFactor = factor; best = box; bestWidth = w; }
  }
  canvas.style.width = bestWidth + 'px';

  scale = Math.min(vw / best.w, vh / best.h, 1);
  panX = (vw - best.w * scale) / 2;
  panY = (vh - best.h * scale) / 2;
  applyTransform();
}

function panToCard(idx) {
  if (idx < 0 || idx >= cards.length) return;
  currentCardIdx = idx;
  cards.forEach((c, i) => c.classList.toggle('latest', i === idx));
  const card = cards[idx];
  const vw = viewport.clientWidth;
  const vh = viewport.clientHeight;
  const cx = card.offsetLeft + card.offsetWidth / 2;
  const cy = card.offsetTop + card.offsetHeight / 2;
  panX = vw / 2 - cx * scale;
  panY = vh / 2 - cy * scale;
  applyTransform();
}

function panToLatest() {
  panToCard(cards.length - 1);
}

function panToAnnotation(idx) {
  if (idx < 0 || idx >= annotations.length) return;
  currentAnnIdx = idx;
  const mark = annotations[idx];
  // Highlight active annotation
  annotations.forEach((m, i) => m.classList.toggle('active', i === idx));
  // Compute delta to center the mark in viewport
  const vRect = viewport.getBoundingClientRect();
  const mRect = mark.getBoundingClientRect();
  panX += viewport.clientWidth  / 2 - (mRect.left + mRect.width  / 2 - vRect.left);
  panY += viewport.clientHeight / 2 - (mRect.top  + mRect.height / 2 - vRect.top);
  applyTransform();
}

// Arrow keys: annotations first, fall back to cards
window.addEventListener('keydown', e => {
  if (e.key === 'ArrowRight') {
    if (annotations.length) panToAnnotation(Math.min(annotations.length - 1, currentAnnIdx + 1));
    else panToCard(Math.min(cards.length - 1, currentCardIdx + 1));
  }
  if (e.key === 'ArrowLeft') {
    if (annotations.length) panToAnnotation(Math.max(0, currentAnnIdx - 1));
    else panToCard(Math.max(0, currentCardIdx - 1));
  }
});

// Apply an LLM-pushed annotation: find searchText in the hljs-rendered code,
// wrap it in a <mark class="annotation">, and add a comment item in annPanel.
function applyAnnotation(container, searchText, comment, annPanel) {
  let foundMark = null;

  function walk(node) {
    if (node.nodeType === Node.TEXT_NODE) {
      const idx = node.nodeValue.indexOf(searchText);
      if (idx === -1) return false;
      const before = node.nodeValue.slice(0, idx);
      const after  = node.nodeValue.slice(idx + searchText.length);
      const mark = document.createElement('mark');
      mark.className = 'annotation';
      mark.setAttribute('data-comment', comment);
      mark.textContent = searchText;
      const parent = node.parentNode;
      if (before) parent.insertBefore(document.createTextNode(before), node);
      parent.insertBefore(mark, node);
      if (after)  parent.insertBefore(document.createTextNode(after), node);
      parent.removeChild(node);
      annotations.push(mark);
      foundMark = mark;
      return true;
    }
    for (const child of Array.from(node.childNodes)) {
      if (walk(child)) return true;
    }
    return false;
  }
  walk(container);

  if (!foundMark || !annPanel) return;

  const item = document.createElement('div');
  item.className = 'ann-item';
  item._mark = foundMark;
  const arrow = document.createElement('span');
  arrow.className = 'ann-arrow';
  arrow.textContent = '→';
  const bubble = document.createElement('span');
  bubble.className = 'ann-comment';
  bubble.textContent = comment;
  item.appendChild(arrow);
  item.appendChild(bubble);
  annPanel.appendChild(item);

  foundMark.onclick = () => {
    const idx = annotations.indexOf(foundMark);
    if (idx >= 0) panToAnnotation(idx);
  };

  return foundMark;
}

// After all annotations are applied, position items to align with marks and de-overlap.
function layoutAnnotations(annPanel) {
  const items = Array.from(annPanel.querySelectorAll('.ann-item'));
  if (!items.length) return;

  const stop = annPanel.parentElement; // card-body

  // Compute ideal top for each item (mark's offsetTop relative to card-body)
  items.forEach(item => {
    let top = 0, el = item._mark;
    while (el && el !== stop) { top += el.offsetTop; el = el.offsetParent; }
    item._idealTop = top;
    item.style.top = top + 'px';
  });

  // Sort by ideal position then de-overlap top-to-bottom
  items.sort((a, b) => a._idealTop - b._idealTop);
  let floor = 0;
  items.forEach(item => {
    const top = Math.max(item._idealTop, floor);
    item.style.top = top + 'px';
    floor = top + item.offsetHeight + 4;
  });

  // Set panel min-height to contain all items
  const last = items[items.length - 1];
  annPanel.style.minHeight = (parseInt(last.style.top) + last.offsetHeight + 12) + 'px';
}

function updateNavHint() {
  const hint = document.getElementById('nav-hint');
  if (annotations.length) {
    hint.textContent = '← → ' + annotations.length + ' annotation' + (annotations.length > 1 ? 's' : '');
    hint.classList.add('has-annotations');
  } else {
    hint.textContent = '← → navigate cards';
    hint.classList.remove('has-annotations');
  }
}

function respond(decision) {
  promptBar.classList.remove('active');
  fetch('/api/respond', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({decision}) });
}
document.getElementById('btn-accept').onclick = () => respond('accept');
document.getElementById('btn-reject').onclick = () => respond('reject');

function connect() {
  const es = new EventSource('/events');
  es.onopen = () => { statusEl.textContent = 'connected'; statusEl.style.color = '#3fb950'; };
  es.onerror = () => {
    statusEl.textContent = 'reconnecting...';
    statusEl.style.color = '#f85149';
    setTimeout(connect, 2000);
    es.close();
  };
  es.onmessage = (ev) => {
    const e = JSON.parse(ev.data);
    if (e.type === 'history') {
      rebuildCanvas(e.events);
      if (e.events.length) fitAll();
    } else if (e.type === 'show' || e.type === 'diff') {
      addCard(e);
    } else if (e.type === 'clear') {
      canvas.innerHTML = ''; cards = [];
      placeholder.style.display = 'flex';
      panX = 0; panY = 0; scale = 1; applyTransform();
    } else if (e.type === 'prompt') {
      promptMessage.textContent = e.message;
      promptBar.classList.add('active');
    } else if (e.type === 'prompt_clear') {
      promptBar.classList.remove('active');
    }
  };
}
connect();
</script>
</body>
</html>
`
