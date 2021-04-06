/* YaNFD - Yet another NDN Forwarding Daemon
 *
 * Copyright (C) 2020-2021 Eric Newberry.
 *
 * This file is licensed under the terms of the MIT License, as found in LICENSE.md.
 */

package face

import (
	"net"
	"os"

	"github.com/eric135/YaNFD/core"
	"github.com/eric135/YaNFD/ndn"
)

// UnixStreamListener listens for incoming Unix stream connections.
type UnixStreamListener struct {
	conn     net.Listener
	localURI *ndn.URI
	nextFD   int // We can't (at least easily) access the actual FD through net.Conn, so we'll make our own
	HasQuit  chan bool
}

// MakeUnixStreamListener constructs a UnixStreamListener.
func MakeUnixStreamListener(localURI *ndn.URI) (*UnixStreamListener, error) {
	localURI.Canonize()
	if !localURI.IsCanonical() || localURI.Scheme() != "unix" {
		return nil, core.ErrNotCanonical
	}

	l := new(UnixStreamListener)
	l.localURI = localURI
	l.nextFD = 1
	l.HasQuit = make(chan bool, 1)
	return l, nil
}

func (l *UnixStreamListener) String() string {
	return "UnixStreamListener, " + l.localURI.String()
}

// Run starts the Unix stream listener.
func (l *UnixStreamListener) Run() {
	// Delete any existing socket
	os.Remove(l.localURI.Path())

	// Create listener
	var err error
	if l.conn, err = net.Listen(l.localURI.Scheme(), l.localURI.Path()); err != nil {
		core.LogFatal(l, "Unable to start Unix stream listener: "+err.Error())
	}

	// Set permissions to allow all local apps to communicate with us
	if err := os.Chmod(l.localURI.Path(), 0777); err != nil {
		core.LogFatal(l, "Unable to change permissions on Unix stream listener: "+err.Error())
	}

	core.LogInfo(l, "Listening")

	// Run accept loop
	for {
		newConn, err := l.conn.Accept()
		if err != nil {
			if err.Error() == "EOF" {
				// Must have failed due to being closed, so quit quietly
			} else {
				core.LogWarn(l, "Unable to accept connection: "+err.Error())
			}
			break
		}

		// Construct remote URI
		remoteURI := ndn.MakeFDFaceURI(l.nextFD)
		l.nextFD++
		if !remoteURI.IsCanonical() {
			core.LogWarn(l, "Unable to create face from "+remoteURI.String()+" as remote URI is not canonical")
			continue
		}

		newTransport, err := MakeUnixStreamTransport(remoteURI, l.localURI, newConn)
		if err != nil {
			core.LogError(l, "Failed to create new Unix stream transport: "+err.Error())
			continue
		}
		newLinkService := MakeNDNLPLinkService(newTransport, MakeNDNLPLinkServiceOptions())
		if err != nil {
			core.LogError(l, "Failed to create new NDNLPv2 transport: "+err.Error())
			continue
		}

		core.LogInfo(l, "Accepting new Unix stream face "+remoteURI.String())

		// Add face to table and start its thread
		FaceTable.Add(newLinkService)
		go newLinkService.Run()
	}

	l.HasQuit <- true
}

// Close closes the UnixStreamListener.
func (l *UnixStreamListener) Close() {
	core.LogInfo(l, "Stopping listener")
	l.conn.Close()
}
