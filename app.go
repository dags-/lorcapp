package lorcapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zserge/lorca"
)

type App struct {
	lorca.UI
	id     *int32
	dir    string
	state  string
	lock   *sync.RWMutex
	bounds lorca.Bounds
	load   []func(a *App)
	ready  []func(a *App)
}

func NewApp(name, url string, width, height int) (*App, error) {
	dir := configDir(name)

	var bounds lorca.Bounds
	e := read(filepath.Join(dir, "Window.json"), &bounds)
	if e == nil {
		width = bounds.Width
		height = bounds.Height
	} else {
		bounds = lorca.Bounds{
			Left:        0,
			Top:         0,
			Width:       width,
			Height:      height,
			WindowState: "normal",
		}
	}

	ui, e := lorca.New(url, dir, width, height, "--enable-extensions")
	if e != nil {
		return nil, e
	}

	e = ui.SetBounds(bounds)
	if e != nil {
		log.Println("position err:", e)
	}

	var i int32 = 0

	app := &App{
		UI:     ui,
		id:     &i,
		dir:    dir,
		bounds: bounds,
		state:  "complete",
		load:   []func(*App){},
		ready:  []func(*App){},
		lock:   &sync.RWMutex{},
	}

	if e != nil {
		log.Println("start err:", e)
	}

	go loop(app)

	return app, nil
}

func (a *App) Load(fn func(a *App)) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.load = append(a.load, fn)
}

func (a *App) Ready(fn func(a *App)) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.ready = append(a.ready, fn)
}

func (a *App) InjectCSS(css string) {
	id := atomic.AddInt32(a.id, 1)
	b := bytes.Buffer{}
	b.WriteString(fmt.Sprintf(`let ___%v=document.createElement("style");`, id))
	b.WriteString(fmt.Sprintf("___%v.innerHTML=`%s`;", id, css))
	b.WriteString(fmt.Sprintf("document.body.appendChild(___%v);", id))
	a.Eval(b.String())
}

func (a *App) InjectScript(script string) {
	id := atomic.AddInt32(a.id, 1)
	b := bytes.Buffer{}
	b.WriteString(fmt.Sprintf(`let ___%v=document.createElement("script");`, id))
	b.WriteString(fmt.Sprintf("___%v.innerHTML=`%s`;", id, script))
	b.WriteString(fmt.Sprintf("document.body.appendChild(___%v);", id))
	a.Eval(b.String())
}

func (a *App) InjectScriptSrc(url string) {
	id := atomic.AddInt32(a.id, 1)
	b := bytes.Buffer{}
	b.WriteString(fmt.Sprintf(`let ___%v=document.createElement("script");`, id))
	b.WriteString(fmt.Sprintf("___%v.src=`%s`;", id, url))
	b.WriteString(fmt.Sprintf("document.body.appendChild(___%v);", id))
	a.Eval(b.String())
}

func (a *App) Wait() {
	<-a.UI.Done()
	a.Dispose()
}

func (a *App) Dispose() {
	a.lock.Lock()
	e := write(filepath.Join(a.dir, "Window.json"), &a.bounds)
	if e != nil {
		log.Println("stop err:", e)
	}
}

func loop(a *App) {
	for {
		checkBounds(a)
		checkState(a)
		time.Sleep(time.Millisecond * 200)
	}
}

func checkBounds(a *App) {
	b, e := a.UI.Bounds()
	if e == nil {
		a.lock.Lock()
		a.bounds = b
		a.lock.Unlock()
	}
}

func checkState(a *App) {
	state := a.Eval("window.document.readyState").String()
	if state != a.state {
		a.state = state
		if state == "interactive" {
			a.lock.Lock()
			defer a.lock.Unlock()
			for _, fn := range a.load {
				fn(a)
			}
		} else if state == "complete" {
			a.lock.Lock()
			defer a.lock.Unlock()
			for _, fn := range a.ready {
				fn(a)
			}
		}
	}
}

func read(path string, i interface{}) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer logClose(f)
	return json.NewDecoder(f).Decode(i)
}

func write(path string, i interface{}) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer logClose(f)
	en := json.NewEncoder(f)
	en.SetIndent("", "  ")
	return en.Encode(i)
}

func logClose(c io.Closer) {
	e := c.Close()
	if e != nil {
		log.Println(e)
	}
}

func configDir(name string) string {
	base := ""
	u, e := user.Current()
	if e == nil {
		base = u.HomeDir
	}
	path := filepath.Join(base, "AppData", "Local", name)
	if _, e := os.Stat(path); e != nil {
		e = os.MkdirAll(path, os.ModePerm)
		if e != nil {
			log.Println(e)
		}
	}
	return path
}
