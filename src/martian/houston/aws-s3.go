//
// Copyright (c) 2015 10X Genomics, Inc. All rights reserved.
//
// Houston AWS S3 downloader.
//

package main

import (
	"fmt"
	"martian/core"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

//const MAXDOWNLOAD = 5 * 1000 * 1000 * 1000 // 5GB
const MAXDOWNLOAD = 10 * 1000 * 1000 // 10MB

type DownloadManager struct {
	bucket       string
	downloadPath string
	storagePath  string
	keyRE        *regexp.Regexp
}

func NewDownloadManager(bucket string, downloadPath string, storagePath string) *DownloadManager {
	self := &DownloadManager{}
	self.bucket = bucket
	self.downloadPath = downloadPath
	self.storagePath = storagePath
	self.keyRE = regexp.MustCompile("^(\\d{4})-(\\d{2})-(\\d{2})-(.*)@(.*)-([A-Z0-9]{5,6})-(.*)$")
	return self
}

func (self *DownloadManager) StartDownloadLoop() {
	go func() {
		for {
			self.download()
			time.Sleep(time.Minute * time.Duration(5))
		}
	}()
}

func (self *DownloadManager) download() {

	// ListObjects in our bucket
	response, err := s3.New(nil).ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(self.bucket),
		Prefix: aws.String("2"),
	})
	if err == nil {
		core.LogInfo("download", "ListObjects returned %d objects", len(response.Contents))
	} else {
		core.LogError(err, "download", "ListObjects failed")
		return
	}

	// Iterate over all returned objects
	for _, object := range response.Contents {
		key := *object.Key
		size := *object.Size
		core.LogInfo("download", "Processing %s", key)

		if size > MAXDOWNLOAD {
			core.LogInfo("download", "    Exceeds maximum size %d > %d", size, MAXDOWNLOAD)
			continue
		}

		// Parse the object key string, must be in expected format generated by miramar
		subs := self.keyRE.FindStringSubmatch(key)
		if len(subs) != 8 {
			core.LogInfo("download", "    Key parse failed, skipping")
			continue
		}

		// Extract key components
		year := subs[1]
		month := subs[2]
		day := subs[3]
		user := subs[4]
		domain := subs[5]
		uid := subs[6]
		fname := subs[7]
		ftype := path.Ext(fname)
		fdir := fmt.Sprintf("%s%s", uid, ftype)

		// Construct permanent storage path
		permPath := path.Join(self.storagePath, year, month, day, domain, user, fdir)

		// Skip if permPath already exists
		if _, err := os.Stat(permPath); err == nil {
			core.LogInfo("download", "    Already in permanent storage, skipping")
			continue
		}

		downloadedFile := path.Join(self.downloadPath, key)

		// Setup the local file
		fd, err := os.Create(downloadedFile)
		if err != nil {
			core.LogError(err, "download", "    Could not create file for download")
			continue
		}
		numBytes, err := s3manager.NewDownloader(nil).Download(fd,
			&s3.GetObjectInput{Bucket: &self.bucket, Key: &key})
		core.LogInfo("download", "    Downloaded %d bytes", numBytes)

		// Read 512 bytes of downloaded file for MIME type detection
		fd.Seek(0, 0)
		var magic []byte
		magic = make([]byte, 512)
		_, err = fd.Read(magic)
		fd.Close()
		if err != nil {
			core.LogError(err, "download", "    Could not read from downloaded file")
			continue
		}
		mimeType := http.DetectContentType(magic)

		// Handling of downloaded file depends on type
		var cmd *exec.Cmd
		if strings.HasPrefix(mimeType, "application/x-gzip") {
			core.LogInfo("download", "    Tar file, untar'ing")
			cmd = exec.Command("tar", "xf", downloadedFile, "-C", permPath)
		} else if strings.HasPrefix(mimeType, "text/plain") {
			core.LogInfo("download", "    Text file, copying")
			cmd = exec.Command("cp", downloadedFile, path.Join(permPath, fname))
		} else {
			core.LogInfo("download", "    Unknown filetype %s", mimeType)
			continue
		}

		// Create permanent storage folder for this key
		if err := os.MkdirAll(permPath, 0755); err != nil {
			core.LogError(err, "download", "    Could not create directory: %s", permPath)
			continue
		}

		// Execute handler command
		if _, err = cmd.Output(); err != nil {
			core.LogError(err, "download", "    Error while running handler")

			// Remove the permPath so this can be retried later
			os.RemoveAll(permPath)
			continue
		}

		// Success! Remove the temporary downloaded file
		os.Remove(downloadedFile)
		core.LogInfo("download", "    Handler complete, removed download file")
	}
}
