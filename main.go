package main

import (
	"Plrx/lib/bot"

	_ "Plrx/plugins/bind"
	_ "Plrx/plugins/echo"
	_ "Plrx/plugins/imagegen"
	_ "Plrx/plugins/uptime"
)

func main() {
	bot.Run()
}
