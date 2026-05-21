package main

import (
	_ "embed"

	"github.com/conantorreswf/ratelash/testserver/handler"
)

//go:embed dashboard/index.html
var dashboardHTML []byte

func init() {
	handler.DashboardHTML = dashboardHTML
}
