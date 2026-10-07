package strongswan

import (
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
)

func TestLazyTemplatesConcurrentFirstUse(t *testing.T) {
	var loader rendererTemplatesLoader
	var calls atomic.Int32
	parse := func() *template.Template { calls.Add(1); return parseTemplates() }
	if loader.load != nil || calls.Load() != 0 {
		t.Fatal("zero loader eagerly initialized")
	}
	const workers = 64
	results := make(chan *template.Template, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() { defer wg.Done(); results <- loader.get(parse) }()
	}
	wg.Wait()
	close(results)
	var first *template.Template
	for result := range results {
		if first == nil {
			first = result
		}
		if result == nil || result != first {
			t.Fatal("concurrent renders received different template sets")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("parser called %d times", calls.Load())
	}
	for _, name := range []string{"ngfw.conf.tmpl", "ngfw-secrets.conf.tmpl", "strongswan.conf.tmpl"} {
		if first.Lookup(name) == nil {
			t.Fatalf("trusted template missing: %s", name)
		}
	}
	if loader.get(func() *template.Template { t.Fatal("later parser called"); return nil }) != first {
		t.Fatal("template pointer changed")
	}
}

func TestLazyTemplatesPanicReplay(t *testing.T) {
	var loader rendererTemplatesLoader
	var calls atomic.Int32
	want := &struct{ reason string }{"trusted parse failure"}
	parse := func() *template.Template { calls.Add(1); panic(want) }
	const workers = 64
	results := make(chan any, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { results <- recover() }()
			loader.get(parse)
		}()
	}
	wg.Wait()
	close(results)
	for got := range results {
		if got != want {
			t.Fatalf("panic value changed: %v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("panicking parser called %d times", calls.Load())
	}
	func() {
		defer func() {
			if recover() != want {
				t.Fatal("sequential panic was not replayed")
			}
		}()
		loader.get(func() *template.Template { t.Fatal("panic cache retried parser"); return nil })
	}()
}
