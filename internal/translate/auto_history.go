package translate

import "cnb.cool/dtapp/kai/internal/model"

// The history of automatic translations (issue #57).
//
// While the user types, the window translates by itself after each pause. Writing every one of those
// would fill the history with half-typed sentences, so an automatic request (TranslateRequest.Auto)
// keeps what its engines produced in memory instead, for the newest such request only. The window
// asks to write it (CommitAutoHistory) once the text has stopped changing. A newer request, automatic
// or not, replaces what was waiting, so a text that was still being typed is never saved.

// stashAutoHistory holds res for request id, whose sequence number is seq. The rows belong to the
// newest request: a result of an older one (a smaller seq) is ignored, and a newer one replaces
// what an older one left. Ordering is by seq, never by when the call happens to arrive, so a slow
// engine of a superseded request that lands late cannot take the rows from the request that
// replaced it.
func (s *Service) stashAutoHistory(id string, seq uint64, res *model.TranslateResult) {
	if res == nil {
		return
	}
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if seq < s.autoSeq {
		return
	}
	if seq > s.autoSeq || s.autoID != id {
		s.autoSeq, s.autoID, s.autoRows = seq, id, nil
	}
	s.autoRows = append(s.autoRows, res)
}

// dropAutoHistory forgets what an automatic request was holding, because the request with sequence
// number seq (one the user asked for) replaces it. Anything older that lands later is ignored.
func (s *Service) dropAutoHistory(seq uint64) {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if seq > s.autoSeq {
		s.autoSeq = seq
	}
	s.autoID, s.autoRows = "", nil
}

// CommitAutoHistory writes to the history what the automatic request id produced, and reports how
// many results it handed to the history (the history itself skips an identical earlier one). It
// writes nothing for a request that a newer one replaced, nor for an unknown one, and a second call
// for the same id writes nothing.
func (s *Service) CommitAutoHistory(id string) int {
	if id == "" {
		return 0
	}
	s.autoMu.Lock()
	if s.autoID != id {
		s.autoMu.Unlock()
		return 0
	}
	rows := s.autoRows
	s.autoID, s.autoRows = "", nil
	s.autoMu.Unlock()
	for _, r := range rows {
		s.saveHistory(r)
	}
	return len(rows)
}
