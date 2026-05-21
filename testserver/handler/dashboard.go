package handler

import "net/http"

var DashboardHTML []byte

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(DashboardHTML)
}
