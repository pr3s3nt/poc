// Observability acceptance workload. Ordinary stdout/stderr and process resource
// use exercise the platform collector without an instrumentation SDK or agent.
package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var until atomic.Int64
var held struct {
	sync.Mutex
	memory []byte
}
var sink atomic.Uint64
var boot = fmt.Sprintf("boot-%x", rand.Uint64())
var revision = os.Getenv("REVISION")

func emit(level, message string) {
	out := os.Stdout
	if level == "ERROR" {
		out = os.Stderr
	}
	_ = json.NewEncoder(out).Encode(map[string]any{"level": level, "message": message, "boot": boot, "revision": revision, "time": time.Now().UTC()})
}
func stopLoad() { until.Store(0); held.Lock(); held.memory = nil; held.Unlock(); debug.FreeOSMemory() }
func main() {
	runtime.GOMAXPROCS(1)
	emit("INFO", "application-started")
	go func() {
		for {
			if until.Load() > time.Now().UnixNano() {
				var n uint64
				for i := 0; i < 500000; i++ {
					n = n*1664525 + uint64(i) + 1013904223
				}
				sink.Store(n)
				runtime.Gosched()
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			if until.Load() != 0 && until.Load() < time.Now().UnixNano() {
				stopLoad()
				emit("INFO", "load-finished")
			}
			emit("INFO", "heartbeat")
		}
	}()
	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	http.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"boot": boot, "revision": revision, "loading": until.Load() > time.Now().UnixNano()})
	})
	http.HandleFunc("POST /action/{action}", func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		switch action {
		case "info":
			emit("INFO", "user-info-marker")
		case "batch":
			for i := 0; i < 120; i++ {
				emit("INFO", fmt.Sprintf("batch-line-%03d", i))
			}
		case "warn":
			emit("WARN", "user-warning-marker")
		case "error":
			emit("ERROR", "user-error-marker")
		case "load":
			held.Lock()
			held.memory = make([]byte, 208<<20)
			for i := 0; i < len(held.memory); i += 4096 {
				held.memory[i] = byte(i / 4096)
			}
			held.Unlock()
			until.Store(time.Now().Add(120 * time.Second).UnixNano())
			emit("WARN", "load-started")
		case "stop":
			stopLoad()
			emit("INFO", "load-stopped")
		case "restart":
			emit("WARN", "user-process-restart")
			go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
		default:
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, action+" accepted")
	})
	http.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html lang="vi"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Ứng dụng kiểm chứng telemetry</title><style>body{font:16px system-ui;max-width:800px;margin:60px auto;padding:24px;color:#182638}button{padding:12px;margin:8px 8px 8px 0;border:1px solid #aaa;border-radius:5px;cursor:pointer}pre{background:#f1f5fa;padding:20px;white-space:pre-wrap}</style><h1>Ứng dụng kiểm chứng telemetry</h1><p>Log stdout/stderr và tải CPU/RAM thực trong workload.</p><button data-action="info">Ghi INFO</button><button data-action="batch">Ghi 120 dòng</button><button data-action="warn">Ghi WARN</button><button data-action="error">Ghi ERROR</button><button data-action="load">Tạo tải CPU/RAM</button><button data-action="stop">Dừng tải</button><button data-action="restart">Khởi động lại tiến trình</button><pre id="status" role="status"></pre><p id="result" role="status"></p><script>async function refresh(){try{document.querySelector('#status').textContent=JSON.stringify(await(await fetch('/status')).json(),null,2)}catch{}}document.querySelectorAll('button').forEach(b=>b.onclick=async()=>{const r=await fetch('/action/'+b.dataset.action,{method:'POST'});document.querySelector('#result').textContent=await r.text();await refresh()});setInterval(refresh,2000);refresh();</script></html>`)
	})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		emit("ERROR", "http-server-stopped")
		os.Exit(1)
	}
}
