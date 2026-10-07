package translate

import (
	"context"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #57: a text translated while the user is still typing must not fill the history with one
// row per pause. An automatic request (Auto) keeps what it produced in memory instead of writing it;
// the window commits it once the text has stopped changing (CommitAutoHistory). A newer request,
// automatic or not, replaces what was waiting, so a half-typed sentence is never saved.

func probeEngine() *fakeEngine {
	return &fakeEngine{name: "probe", run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: "translated " + req.Text, From: req.From, To: req.To, Text: req.Text}, nil
	}}
}

func autoReq(id, text string, auto bool) model.TranslateRequest {
	return model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: id, Auto: auto}
}

func settled(t *testing.T, em *recEmitter, id string) {
	t.Helper()
	waitUntil(t, "result of "+id, func() bool { return len(em.resultsFor(id, "probe")) == 1 })
}

func TestAutoTranslationWritesNoHistoryUntilCommitted(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	if _, err := svc.TranslateMulti(autoReq("a1", "frase automatica uno", true)); err != nil {
		t.Fatal(err)
	}
	settled(t, em, "a1")
	// waitUntil returns when the result is emitted; the history write, if any, happens before that.
	if n := historyRows(t, hist, "automatica"); n != 0 {
		t.Fatalf("%d rows before the commit, want 0", n)
	}
	if got := svc.CommitAutoHistory("a1"); got != 1 {
		t.Fatalf("CommitAutoHistory saved %d rows, want 1", got)
	}
	if n := historyRows(t, hist, "automatica"); n != 1 {
		t.Fatalf("%d rows after the commit, want 1", n)
	}
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("a second commit saved %d rows, want 0 (it is cleared)", got)
	}
}

func TestANewerAutoRequestReplacesWhatWasWaiting(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	_, _ = svc.TranslateMulti(autoReq("a1", "mitad de una fra", true))
	settled(t, em, "a1")
	_, _ = svc.TranslateMulti(autoReq("a2", "mitad de una frase completa", true))
	settled(t, em, "a2")
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("the replaced request saved %d rows, want 0", got)
	}
	if got := svc.CommitAutoHistory("a2"); got != 1 {
		t.Errorf("the newest saved %d rows, want 1", got)
	}
	if n := historyRows(t, hist, "mitad"); n != 1 {
		t.Errorf("%d rows mention the text, want exactly the complete one", n)
	}
	if n := historyRows(t, hist, "fra "); n != 0 {
		t.Errorf("the half-typed text was saved")
	}
}

func TestAManualTranslationDropsWhatWasWaitingAndSavesItself(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	_, _ = svc.TranslateMulti(autoReq("a1", "texto escrito a medias", true))
	settled(t, em, "a1")
	_, _ = svc.TranslateMulti(autoReq("m1", "texto escrito del todo", false))
	settled(t, em, "m1")
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("committing the dropped request saved %d rows, want 0", got)
	}
	if n := historyRows(t, hist, "del todo"); n != 1 {
		t.Errorf("the manual translation wrote %d rows, want 1", n)
	}
	if n := historyRows(t, hist, "a medias"); n != 0 {
		t.Errorf("the dropped automatic text was saved")
	}
}

func TestCommitOfAnUnknownRequestSavesNothing(t *testing.T) {
	svc, _, _ := newCancelService(t, probeEngine())
	if got := svc.CommitAutoHistory("nope"); got != 0 {
		t.Errorf("saved %d rows for an unknown request", got)
	}
	if got := svc.CommitAutoHistory(""); got != 0 {
		t.Errorf("saved %d rows for an empty id", got)
	}
}
