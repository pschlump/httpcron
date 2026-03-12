package config

// Copyright (C) Philip Schlump, 2025.

import (
	"fmt"
	"os"
	"testing"

	"github.com/pschlump/dbgo"
)

func TestReadConfig(t *testing.T) {

	cfg, err := FromFile("./data.test-config.json")

	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}

	got := dbgo.SVarI(cfg)

	tmp, err := os.ReadFile("./ref/cfg.out")
	if err != nil {
		t.Fatal(err)
	}
	expect := string(tmp)

	if got != expect {
		fmt.Printf("%s\n", got)
		t.Errorf("Got >%s< did not match >%s<\n", got, expect)
		err = os.WriteFile("./out/cfg.out", []byte(got), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	if db {
		dbgo.Printf("Success: >%s< \n", got)
	}

}

var db = false

/* vim: set noai ts=4 sw=4: */
