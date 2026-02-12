package common

import (
	"strconv"
	"strings"
	"sync"
)

// RequestLogOnlyUsersRaw supports comma-separated usernames or user IDs.
// Examples: "alice,bob,1001,1002"
var RequestLogOnlyUsersRaw = ""

var requestLogOnlyUserIDs = map[int]struct{}{}
var requestLogOnlyUsernames = map[string]struct{}{}
var requestLogOnlyUsersRWMutex sync.RWMutex

// RequestLogOnlyUsersEnabled indicates whether user filter is active.
var RequestLogOnlyUsersEnabled = false

func InitRequestLogOnlyUsers(raw string) {
	normalized := strings.TrimSpace(raw)

	requestLogOnlyUsersRWMutex.Lock()
	defer requestLogOnlyUsersRWMutex.Unlock()

	if normalized == RequestLogOnlyUsersRaw {
		return
	}

	RequestLogOnlyUsersRaw = normalized
	requestLogOnlyUserIDs = make(map[int]struct{})
	requestLogOnlyUsernames = make(map[string]struct{})
	RequestLogOnlyUsersEnabled = false

	if RequestLogOnlyUsersRaw == "" {
		return
	}

	for _, item := range strings.Split(RequestLogOnlyUsersRaw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if userID, err := strconv.Atoi(value); err == nil && userID > 0 {
			requestLogOnlyUserIDs[userID] = struct{}{}
			RequestLogOnlyUsersEnabled = true
			continue
		}
		requestLogOnlyUsernames[strings.ToLower(value)] = struct{}{}
		RequestLogOnlyUsersEnabled = true
	}
}

func ShouldRecordRequestLogForUser(userID int, username string) bool {
	requestLogOnlyUsersRWMutex.RLock()
	defer requestLogOnlyUsersRWMutex.RUnlock()

	if !RequestLogOnlyUsersEnabled {
		return true
	}

	if userID > 0 {
		if _, ok := requestLogOnlyUserIDs[userID]; ok {
			return true
		}
	}

	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return false
	}
	_, ok := requestLogOnlyUsernames[username]
	return ok
}
