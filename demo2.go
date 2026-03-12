package main

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

func main2() {
	// Initialize with seconds support (optional, default is 5 fields)
	c := cron.New(cron.WithSeconds())

	// Equivalent to: "every 5 seconds"
	c.AddFunc("@every 5s", func() {
		fmt.Println("Tick:", time.Now())
	})

	// Equivalent to: "every 1 hour 30 minutes"
	c.AddFunc("@every 1h30m", func() {
		fmt.Println("Hourly 30m task")
	})

	c.Start()
	// Keep application running
	select {}
}
