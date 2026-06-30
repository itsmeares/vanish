package secretstore

import "strings"

const defaultService = "vanish"

func RedditRefreshKey(username string) Key {
	username = strings.ToLower(strings.TrimSpace(username))
	return Key{
		Service: defaultService + "/reddit",
		Account: "refresh:" + username,
	}
}
