// Package store is a sample with planted issues for fixture capture.
package store

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// FindUser builds SQL from user input.
func FindUser(db *sql.DB, r *http.Request) (*sql.Rows, error) {
	id := r.URL.Query().Get("id")
	query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id)
	return db.Query(query)
}

// Close ignores the rollback error.
func Close(tx *sql.Tx) {
	tx.Rollback()
}

// Title extracts a page title.
func Title(page string) string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	return doc.Data
}

// Grade has many branches.
func Grade(score int) string {
	if score > 90 {
		return "A"
	} else if score > 80 {
		return "B"
	} else if score > 70 {
		return "C"
	} else if score > 60 {
		return "D"
	}
	return "F"
}
