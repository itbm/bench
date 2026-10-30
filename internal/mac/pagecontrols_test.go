package mac

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/platform"
)

func TestCaptureStates(t *testing.T) {
	for _, s := range []platform.CaptureState{platform.CaptureNone, platform.CaptureActive, platform.CaptureMuted} {
		code, ok := captureCode(s)
		if !ok {
			t.Errorf("captureCode(%q) isn't a code", s)
			continue
		}
		if got := captureStateFor(code); got != s {
			t.Errorf("captureStateFor(captureCode(%q)) = %q", s, got)
		}
	}
	if _, ok := captureCode("paused"); ok {
		t.Error("captureCode(\"paused\") should report false")
	}
	// A state that WebKit may add later counts as none, not as in use.
	if got := captureStateFor(7); got != platform.CaptureNone {
		t.Errorf("captureStateFor(7) = %q, want none", got)
	}
	for d, want := range map[platform.CaptureDevice]mediaKind{platform.Camera: mediaCamera, platform.Microphone: mediaMicrophone} {
		if got, ok := deviceKind(d); !ok || got != want {
			t.Errorf("deviceKind(%q) = %v, %v, want %v", d, got, ok, want)
		}
	}
	if _, ok := deviceKind("speaker"); ok {
		t.Error("deviceKind(\"speaker\") should report false")
	}
	want := platform.WebState{CanGoBack: true, Camera: platform.CaptureMuted, Microphone: platform.CaptureActive}
	if got := webState(true, false, captureMuted, captureActive); got != want {
		t.Errorf("webState() = %+v, want %+v", got, want)
	}
}

func TestFindsHandOverAnswers(t *testing.T) {
	var f finds
	first, firstAnswer, ok := f.ask()
	second, secondAnswer, _ := f.ask()
	if !ok || first == second {
		t.Fatalf("ask() = %d and %d, %v, want two requests", first, second, ok)
	}
	// Answers can come in any order, from another thread, without waiting.
	var wg sync.WaitGroup
	wg.Go(func() {
		f.answer(second, false)
		f.answer(first, true)
		f.answer(first, false) // A second answer is dropped.
		f.answer(99, true)     // So is one for a request nobody waits for.
	})
	wg.Wait()
	if found, err := f.wait(first, firstAnswer, time.Second); !found || err != nil {
		t.Errorf("wait(first) = %v, %v, want found", found, err)
	}
	if found, err := f.wait(second, secondAnswer, time.Second); found || err != nil {
		t.Errorf("wait(second) = %v, %v, want not found", found, err)
	}
}

func TestFindsTimeOutAndClose(t *testing.T) {
	var f finds
	request, answer, _ := f.ask()
	if _, err := f.wait(request, answer, time.Millisecond); !errors.Is(err, errFindTimeout) {
		t.Errorf("wait() with no answer = %v, want errFindTimeout", err)
	}
	f.answer(request, true) // Too late: nobody waits, and it mustn't block.
	if len(f.waiting) != 0 {
		t.Errorf("%d requests still wait after the timeout", len(f.waiting))
	}

	request, answer, _ = f.ask()
	f.close()
	f.close()
	if _, err := f.wait(request, answer, time.Second); !errors.Is(err, errFindClosed) {
		t.Errorf("wait() after close = %v, want errFindClosed", err)
	}
	if _, _, ok := f.ask(); ok {
		t.Error("ask() after close should report false")
	}
}
