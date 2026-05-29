package main

import (
	_ "embed"

	"github.com/conantorreswf/limithit/testserver/handler"
)

//go:embed dashboard/index.html
var dashboardHTML []byte

func init() {
	handler.DashboardHTML = dashboardHTML
}
