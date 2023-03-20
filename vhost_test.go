package main

import (
	"testing"
)

func TestVhostParsing(t *testing.T) {
	vhost, err := vhostFromHostname("localhost")
	if err == nil || vhost != "" {
		t.Errorf("Expected an error when serving %s, got nothing", "localhost")
	}

	vhost, err = vhostFromHostname("fff.red")
	if err == nil || vhost != "" {
		t.Errorf("Expected an error when serving %s, got nothing and %s", "fff.red", vhost)
	}

	vhost, err = vhostFromHostname("wtf.fff.red")
	if err != nil || vhost != "wtf" {
		t.Errorf("Expected %s when serving %s, got %s and %v", "wtf", "wtf.fff.red", vhost, err)
	}

	// May want to change this later, to support
	// deeper levels of vhosts, but ignore for now
	vhost, err = vhostFromHostname("hello.world.fff.red")
	if err != nil || vhost != "hello" {
		t.Errorf("Expected %s when serving %s, got %s and %v", "hello", "hello.world.fff.red", vhost, err)
	}
}

