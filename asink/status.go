/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

var localUpdates int32 = 0
var remoteUpdates int32 = 0
var fileUploads int32 = 0
var fileDownloads int32 = 0
var sendingUpdates int32 = 0
var onofflineSince int64 = 0 //low bit is 1 if online, 0 if offline

func GetStats() string {
	local := atomic.LoadInt32(&localUpdates)
	remote := atomic.LoadInt32(&remoteUpdates)
	uploads := atomic.LoadInt32(&fileUploads)
	downloads := atomic.LoadInt32(&fileDownloads)
	onoffline := atomic.LoadInt64(&onofflineSince)
	onoff := "On"
	if onoffline&1 == 0 {
		onoff = "Off"
	}
	onoffSinceTime := time.Unix(0, onoffline)

	return fmt.Sprintf(`Asink client statistics:
	Processing %d file updates (%d local, %d remote)
	Uploading %d files
	Downloading %d files
	Sending %d updates
	%sline since %s`, local+remote, local, remote, uploads, downloads, sendingUpdates, onoff, onoffSinceTime.Format(time.RFC1123))
}

func StatStartLocalUpdate() {
	atomic.AddInt32(&localUpdates, 1)
}
func StatStopLocalUpdate() {
	atomic.AddInt32(&localUpdates, -1)
}
func StatStartRemoteUpdate() {
	atomic.AddInt32(&remoteUpdates, 1)
}
func StatStopRemoteUpdate() {
	atomic.AddInt32(&remoteUpdates, -1)
}
func StatStartUpload() {
	atomic.AddInt32(&fileUploads, 1)
}
func StatStopUpload() {
	atomic.AddInt32(&fileUploads, -1)
}
func StatStartDownload() {
	atomic.AddInt32(&fileDownloads, 1)
}
func StatStopDownload() {
	atomic.AddInt32(&fileDownloads, -1)
}
func StatStartSending() {
	atomic.AddInt32(&sendingUpdates, 1)
}
func StatStopSending() {
	atomic.AddInt32(&sendingUpdates, -1)
}
func StatOnline() {
	unixNano := atomic.LoadInt64(&onofflineSince)
	if unixNano == 0 || unixNano&1 == 0 {
		unixNano := time.Now().UnixNano() | 1
		atomic.StoreInt64(&onofflineSince, unixNano)
	}
}
func StatOffline() {
	unixNano := atomic.LoadInt64(&onofflineSince)
	if unixNano == 0 || unixNano&1 != 0 {
		unixNano = time.Now().UnixNano() & ^1
		atomic.StoreInt64(&onofflineSince, unixNano)
	}
}
