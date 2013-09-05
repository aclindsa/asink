/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"fmt"
	"sync/atomic"
)

var localUpdates int32 = 0
var remoteUpdates int32 = 0
var fileUploads int32 = 0
var fileDownloads int32 = 0
var sendingUpdates int32 = 0

func GetStats() string {
	local := atomic.LoadInt32(&localUpdates)
	remote := atomic.LoadInt32(&remoteUpdates)
	uploads := atomic.LoadInt32(&fileUploads)
	downloads := atomic.LoadInt32(&fileDownloads)

	return fmt.Sprintf(`Asink client statistics:
	Processing %d file updates (%d local, %d remote)
	Uploading %d files
	Downloading %d files
	Sending %d updates`, local+remote, local, remote, uploads, downloads, sendingUpdates)
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
