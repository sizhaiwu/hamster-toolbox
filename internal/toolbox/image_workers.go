package toolbox

import (
	"context"
	"path/filepath"
	"sync"
)

func executeParallelFiles(ctx context.Context, p *Plan, log func(string), progress func(int, int)) Result {
	var mu sync.Mutex
	safeLog := func(message string) { mu.Lock(); defer mu.Unlock(); log(message) }
	sessions := sync.Pool{New: func() any { return newVideoSession(p.Config, safeLog) }}
	return parallelFileItems(ctx, p.Items, p.Config.Workers, func(it Item) Result {
		one := *p
		one.Items = []Item{it}
		itemLog := func(line string) { safeLog("[" + filepath.Base(it.Source) + "] " + line) }
		var session *videoSession
		if p.Config.Tool == "video" {
			session = sessions.Get().(*videoSession)
			session.log = itemLog
			defer sessions.Put(session)
		}
		return executeFiles(ctx, &one, itemLog, func(int, int) {}, false, session)
	}, progress)
}

func parallelFileItems(ctx context.Context, items []Item, workers int, execute func(Item) Result, progress func(int, int)) Result {
	workers = min(max(workers, 1), 8, len(items))
	if workers == 0 {
		return Result{}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	result := Result{}
	done := 0
	jobs := make(chan Item)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := execute(it)
				mu.Lock()
				result.Success += r.Success
				result.Failed += r.Failed
				result.Skipped += r.Skipped
				done++
				progress(done, len(items))
				mu.Unlock()
			}
		}()
	}
dispatch:
	for _, it := range items {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- it:
		}
	}
	close(jobs)
	wg.Wait()
	return result
}
