package utils

import (
	"net/url"
)

func IsValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

/* vim: set noai ts=4 sw=4: */
