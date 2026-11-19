package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// Page change actions, as PageChange.Action names them.
const (
	PageCreate = "create"
	PageEdit   = "edit"
	PageDelete = "delete"
	PageMove   = "move"
)

// PageChange describes one page that a handler has written under the
// documents folder.
type PageChange struct {
	// Action is PageCreate, PageEdit, PageDelete or PageMove.
	Action string
	// Path is the page's path below the documents folder, with no leading
	// slash, such as "personal/cooking/bread".
	Path string
	// OldPath is the page's path before a move. It is empty for other actions.
	OldPath string
}

var (
	pageChangeMu   sync.RWMutex
	pageChangeHook func(PageChange) error
)

// SetPageChangeHook sets the function that the write handlers call after each
// page change under the documents folder. A nil function turns the calls off.
func SetPageChangeHook(f func(PageChange) error) {
	pageChangeMu.Lock()
	defer pageChangeMu.Unlock()
	pageChangeHook = f
}

// notifyPageChange calls the page change hook for a page that a handler has
// written. The paths are logical document paths, such as "/personal/a". The
// home page ("/") is not under the documents folder and is not reported. It
// returns the hook's error.
func notifyPageChange(action, logicalPath, oldLogicalPath string) error {
	pageChangeMu.RLock()
	hook := pageChangeHook
	pageChangeMu.RUnlock()
	if hook == nil {
		return nil
	}
	change := PageChange{
		Action:  action,
		Path:    strings.Trim(logicalPath, "/"),
		OldPath: strings.Trim(oldLogicalPath, "/"),
	}
	if change.Path == "" {
		return nil
	}
	return hook(change)
}

// pageChangeRejected answers 422 with the error's text when err is not nil,
// and reports whether it did. The handler must then write nothing else.
func pageChangeRejected(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": err.Error(),
	})
	return true
}
