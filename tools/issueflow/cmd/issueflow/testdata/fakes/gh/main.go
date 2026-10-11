// Fake `gh` for the CLI smoke test: a JSON file is the whole GitHub.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type issue struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Author string   `json:"author"`
	Labels []string `json:"labels"`
}
type comment struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Body  string `json:"body"`
}
type db struct {
	Issues   []issue              `json:"issues"`
	Comments map[string][]comment `json:"comments"`
	Releases map[string]bool      `json:"releases"`
	NextID   int                  `json:"next_id"`
	Writes   []string             `json:"writes"`
}

var dir = os.Getenv("FAKE_DIR")

func load() *db {
	d := &db{Comments: map[string][]comment{}, Releases: map[string]bool{}, NextID: 100}
	if b, err := os.ReadFile(filepath.Join(dir, "gh.json")); err == nil {
		json.Unmarshal(b, d)
	}
	return d
}
func (d *db) save() {
	b, _ := json.MarshalIndent(d, "", " ")
	os.WriteFile(filepath.Join(dir, "gh.json"), b, 0o644)
}
func fail(code int, msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(code) }

func main() {
	a := os.Args[1:]
	d := load()
	switch {
	case a[0] == "api" && strings.Contains(a[len(a)-1], "/issues?"):
		var out []map[string]any
		for _, i := range d.Issues {
			var ls []map[string]string
			for _, l := range i.Labels {
				ls = append(ls, map[string]string{"name": l})
			}
			out = append(out, map[string]any{"number": i.Number, "title": i.Title, "body": i.Body, "state": "open", "user": map[string]string{"login": i.Author}, "labels": ls})
		}
		json.NewEncoder(os.Stdout).Encode(out)
	case a[0] == "api" && strings.HasSuffix(a[len(a)-1], "/events"):
		fmt.Println(`[{"event":"labeled","actor":{"login":"owner"},"label":{"name":"agent:ready"}}]`)
	case a[0] == "api" && strings.Contains(strings.Join(a, " "), "-X POST"):
		path := ""
		body := ""
		for i, x := range a {
			if strings.Contains(x, "/comments") {
				path = x
			}
			if x == "-f" && i+1 < len(a) {
				body = strings.TrimPrefix(a[i+1], "body=")
			}
		}
		n := strings.Split(path, "/")[4]
		d.NextID++
		d.Comments[n] = append(d.Comments[n], comment{ID: d.NextID, Login: "bot", Body: body})
		d.Writes = append(d.Writes, "comment #"+n)
		d.save()
		fmt.Printf(`{"id":%d}`+"\n", d.NextID)
	case a[0] == "api" && strings.HasSuffix(a[len(a)-1], "/comments"):
		n := strings.Split(a[len(a)-1], "/")[4]
		var out []map[string]any
		for _, c := range d.Comments[n] {
			out = append(out, map[string]any{"id": c.ID, "body": c.Body, "user": map[string]string{"login": c.Login}, "created_at": "2026-01-01T00:00:00Z"})
		}
		json.NewEncoder(os.Stdout).Encode(out)
	case a[0] == "issue" && a[1] == "view":
		for _, i := range d.Issues {
			if fmt.Sprint(i.Number) == a[2] {
				var ls []map[string]string
				for _, l := range i.Labels {
					ls = append(ls, map[string]string{"name": l})
				}
				json.NewEncoder(os.Stdout).Encode(map[string]any{"number": i.Number, "title": i.Title, "body": i.Body, "state": "OPEN", "author": map[string]string{"login": i.Author}, "labels": ls})
				return
			}
		}
		fail(1, "issue not found")
	case a[0] == "issue" && a[1] == "edit":
		for k := range d.Issues {
			if fmt.Sprint(d.Issues[k].Number) != a[2] {
				continue
			}
			for j, x := range a {
				if x == "--add-label" {
					d.Issues[k].Labels = append(d.Issues[k].Labels, a[j+1])
				}
				if x == "--remove-label" {
					var keep []string
					for _, l := range d.Issues[k].Labels {
						if l != a[j+1] {
							keep = append(keep, l)
						}
					}
					d.Issues[k].Labels = keep
				}
			}
			d.Writes = append(d.Writes, "label #"+a[2])
		}
		d.save()
	case a[0] == "release" && a[1] == "view":
		tag := a[2]
		if !d.Releases[tag] {
			fail(1, "release not found")
		}
		if strings.Contains(strings.Join(a, " "), "assets") {
			var as []map[string]any
			ents, _ := os.ReadDir(filepath.Join(dir, "assets", tag))
			for _, e := range ents {
				st, _ := e.Info()
				as = append(as, map[string]any{"name": e.Name(), "size": st.Size(), "url": "https://fake.invalid/" + tag + "/" + e.Name()})
			}
			json.NewEncoder(os.Stdout).Encode(map[string]any{"assets": as})
			return
		}
		fmt.Printf(`{"tagName":%q,"url":"https://fake.invalid/releases/%s","isPrerelease":true}`+"\n", tag, tag)
	case a[0] == "release" && a[1] == "create":
		d.Releases[a[2]] = true
		d.Writes = append(d.Writes, "release "+a[2])
		d.save()
	case a[0] == "release" && a[1] == "upload":
		for _, x := range a {
			if x == "--clobber" {
				fail(2, "fake gh: --clobber must never be used")
			}
		}
		tag, src := a[2], a[len(a)-1]
		if strings.HasPrefix(src, "-") {
			src = a[3]
		}
		dst := filepath.Join(dir, "assets", tag, filepath.Base(src))
		if _, err := os.Stat(dst); err == nil {
			fail(1, "asset already exists")
		}
		os.MkdirAll(filepath.Dir(dst), 0o755)
		in, err := os.Open(src)
		if err != nil {
			fail(1, err.Error())
		}
		out, _ := os.Create(dst)
		io.Copy(out, in)
		out.Close()
		d.Writes = append(d.Writes, "upload "+filepath.Base(src))
		d.save()
	default:
		fail(9, "fake gh: unsupported "+strings.Join(a, " "))
	}
}
