package db

import (
	"regexp"

	"github.com/lib/pq"
)

var uuidRegex = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

func IsValidUUID(u string) bool {
	return uuidRegex.MatchString(u)
}

func IsNonUniqueErr(err error) bool {
	if err, ok := err.(*pq.Error); ok {
		if err.Code == "23505" {
			return true
		}
	}
	return false
}
