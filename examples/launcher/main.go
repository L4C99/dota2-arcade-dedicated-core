// launcher demonstrates an external caller, not a platform or node controller.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/client"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func invoke(c *client.Client, method string, params any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := c.Call(ctx, method, params)
	if e != nil {
		return nil, e
	}
	var v map[string]any
	e = json.Unmarshal(b, &v)
	return v, e
}
func await(c *client.Client, id string) error {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		v, e := invoke(c, "operation", map[string]string{"operationId": id})
		if e != nil {
			return e
		}
		switch v["status"] {
		case "succeeded":
			return nil
		case "failed", "cancelled":
			return fmt.Errorf("operation terminal result: %v", v)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("operation wait timed out; query operation %s before another action", id)
}
func run() error {
	dir := flag.String("data-dir", "", "absolute core data directory")
	template := flag.String("template", "", "absolute template path")
	key := flag.String("key", "", "stable unique intent key; reuse exactly after uncertain failure")
	address := flag.String("connect-host", "127.0.0.1", "client-reachable host; never inferred from wildcard listeners")
	port := flag.Int("port", 0, "optional explicit game port")
	hold := flag.Duration("hold", 30*time.Second, "duration to keep ready room before stop")
	flag.Parse()
	if *template == "" || *key == "" || flag.NArg() != 0 || *hold < 0 || *hold > 24*time.Hour {
		return fmt.Errorf("provide --data-dir ABS --template ABS --key UNIQUE [--connect-host HOST] [--hold 30s]")
	}
	c, e := client.New(*dir)
	if e != nil {
		return e
	}
	params := map[string]any{"template": *template, "port": *port, "idempotencyKey": *key}
	var accepted map[string]any
	for attempt := 0; attempt < 3; attempt++ {
		accepted, e = invoke(c, "create", params)
		if e == nil {
			break
		}
		var rejected *client.Error
		if errors.As(e, &rejected) {
			return e
		}
		// A lost response is not a new creation intent. Every retry is identical.
		time.Sleep(250 * time.Millisecond)
	}
	if e != nil {
		return fmt.Errorf("create outcome unknown; retain key %q and retry identical parameters: %w", *key, e)
	}
	id, ok := accepted["instanceId"].(string)
	if !ok {
		return fmt.Errorf("missing instanceId")
	}
	op, ok := accepted["operationId"].(string)
	if !ok {
		return fmt.Errorf("missing operationId; query instance %s", id)
	}
	fmt.Printf("instance=%s operation=%s key=%s\n", id, op, *key)
	if e = await(c, op); e != nil {
		return e
	}
	state, e := invoke(c, "status", map[string]string{"instanceId": id})
	if e != nil {
		return e
	}
	if state["room"] != "ready" || state["process"] != "running" {
		return fmt.Errorf("historical create succeeded but current room is unavailable: %v", state)
	}
	fmt.Printf("connect %s:%.0f\n", *address, state["port"])
	logs, e := invoke(c, "logs", map[string]any{"instanceId": id, "tail": 5})
	if e != nil {
		return e
	}
	b, _ := json.Marshal(logs)
	fmt.Printf("logs=%s\n", b)
	time.Sleep(*hold)
	stopped, e := invoke(c, "stop", map[string]string{"instanceId": id})
	if e != nil {
		return fmt.Errorf("stop outcome uncertain; retry stop for %s: %w", id, e)
	}
	stopID, ok := stopped["operationId"].(string)
	if !ok {
		return fmt.Errorf("missing stop operationId")
	}
	if e = await(c, stopID); e != nil {
		return e
	}
	state, e = invoke(c, "status", map[string]string{"instanceId": id})
	if e != nil {
		return e
	}
	if state["lifecycle"] != "reclaimed" || state["cleanup"] != "complete" {
		return fmt.Errorf("stop did not reclaim: %v", state)
	}
	fmt.Printf("reclaimed instance=%s stopOperation=%s\n", id, stopID)
	return nil
}
