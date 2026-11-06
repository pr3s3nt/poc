// This isolated acceptance workload emits metrics from actual HTTP requests and
// records real Alertmanager webhook deliveries. It is not an MCP implementation.
package main

import (
 "encoding/json"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "strconv"
 "sync"
 "sync/atomic"
 "time"
)
var fail atomic.Bool
var sequence atomic.Uint64
var mu sync.Mutex
var requests=map[int]uint64{200:0,503:0}
var latencyCount uint64
var latencySum float64
var buckets=map[float64]uint64{0.005:0,0.01:0,0.1:0,1:0}
var receipts=[]json.RawMessage{}
func main(){
 mux:=http.NewServeMux()
 mux.HandleFunc("/health",func(w http.ResponseWriter,r *http.Request){if fail.Load(){w.WriteHeader(503)};fmt.Fprintln(w,"health")})
 mux.HandleFunc("/control",func(w http.ResponseWriter,r *http.Request){if r.Method!="POST"{w.WriteHeader(405);return};fail.Store(r.URL.Query().Get("fail")=="true");fmt.Fprintln(w,"updated")})
 mux.HandleFunc("/receive",func(w http.ResponseWriter,r *http.Request){if r.Method!="POST"{w.WriteHeader(405);return};body,e:=io.ReadAll(io.LimitReader(r.Body,65537));if e!=nil||len(body)>65536||!json.Valid(body){w.WriteHeader(400);return};mu.Lock();receipts=append(receipts,json.RawMessage(body));if len(receipts)>100{receipts=receipts[1:]};mu.Unlock();w.WriteHeader(200)})
 mux.HandleFunc("/receipts",func(w http.ResponseWriter,r *http.Request){mu.Lock();defer mu.Unlock();w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(receipts)})
 mux.HandleFunc("/metrics",func(w http.ResponseWriter,r *http.Request){mu.Lock();defer mu.Unlock();w.Header().Set("Content-Type","text/plain; version=0.0.4");fmt.Fprintln(w,"# TYPE fixture_requests_total counter");for _,code:=range []int{200,503}{fmt.Fprintf(w,"fixture_requests_total{code=%q} %d\n",strconv.Itoa(code),requests[code])};fmt.Fprintln(w,"# TYPE fixture_request_duration_seconds histogram");for _,bound:=range []float64{0.005,0.01,0.1,1}{fmt.Fprintf(w,"fixture_request_duration_seconds_bucket{le=%q} %d\n",strconv.FormatFloat(bound,'g',-1,64),buckets[bound])};fmt.Fprintf(w,"fixture_request_duration_seconds_bucket{le=\"+Inf\"} %d\nfixture_request_duration_seconds_count %d\nfixture_request_duration_seconds_sum %g\n",latencyCount,latencyCount,latencySum);f:=0;if fail.Load(){f=1};fmt.Fprintf(w,"# TYPE fixture_failure gauge\nfixture_failure %d\n",f)})
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){started:=time.Now();code:=200;level:="INFO";if fail.Load(){code=503;level="ERROR"};id:=sequence.Add(1);w.WriteHeader(code);fmt.Fprintf(w,"request %d status %d\n",id,code);elapsed:=time.Since(started).Seconds();mu.Lock();requests[code]++;latencyCount++;latencySum+=elapsed;for b:=range buckets{if elapsed<=b{buckets[b]++}};mu.Unlock();line,_:=json.Marshal(map[string]any{"level":level,"message":"actual HTTP request","request_id":fmt.Sprint(id),"status":code});fmt.Println(string(line));fmt.Println("plain fixture request "+fmt.Sprint(id))})
 port:=os.Getenv("PORT");if port==""{port="8080"};log.Fatal(http.ListenAndServe(":"+port,mux))
}
