// Copyright 2014 go-dockerclient authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
)

// This work with api verion < v1.7 and > v1.9
type APIImages struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:",omitempty"`
	Created     int64
	Size        int64
	VirtualSize int64
	ParentId    string `json:",omitempty"`
	Repository  string `json:",omitempty"`
	Tag         string `json:",omitempty"`
}

// Error returned when the image does not exist.
var (
	ErrNoSuchImage         = errors.New("No such image")
	ErrMissingRepo         = errors.New("Missing remote repository e.g. 'github.com/user/repo'")
	ErrMissingOutputStream = errors.New("Missing output-stream")
)

// ListImages returns the list of available images in the server.
//
// See http://goo.gl/dkMrwP for more details.
func (c *Client) ListImages(all bool) ([]APIImages, error) {
	path := "/images/json?all="
	if all {
		path += "1"
	} else {
		path += "0"
	}
	body, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var images []APIImages
	err = json.Unmarshal(body, &images)
	if err != nil {
		return nil, err
	}
	return images, nil
}

// RemoveImage removes an image by its name or ID.
//
// See http://goo.gl/7hjHHy for more details.
func (c *Client) RemoveImage(name string) error {
	_, status, err := c.do("DELETE", "/images/"+name, nil)
	if status == http.StatusNotFound {
		return ErrNoSuchImage
	}
	return err
}

// InspectImage returns an image by its name or ID.
//
// See http://goo.gl/pHEbma for more details.
func (c *Client) InspectImage(name string) (*Image, error) {
	body, status, err := c.do("GET", "/images/"+name+"/json", nil)
	if status == http.StatusNotFound {
		return nil, ErrNoSuchImage
	}
	if err != nil {
		return nil, err
	}
	var image Image
	err = json.Unmarshal(body, &image)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

// PushImageOptions represents options to use in the PushImage method.
//
// See http://goo.gl/GBmyhc for more details.
type PushImageOptions struct {
	// Name of the image
	Name string

	// Registry server to push the image
	Registry string

	OutputStream io.Writer `qs:"-"`
}

// AuthConfiguration represents authentication options to use in the PushImage
// method. It represents the authencation in the Docker index server.
type AuthConfiguration struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Email    string `json:"email,omitempty"`
}

// PushImage pushes an image to a remote registry, logging progress to w.
//
// An empty instance of AuthConfiguration may be used for unauthenticated
// pushes.
//
// See http://goo.gl/GBmyhc for more details.
func (c *Client) PushImage(opts PushImageOptions, auth AuthConfiguration) error {
	if opts.Name == "" {
		return ErrNoSuchImage
	}
	name := opts.Name
	opts.Name = ""
	path := "/images/" + name + "/push?" + queryString(&opts)
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(auth)
	return c.stream("POST", path, &buf, opts.OutputStream)
}

// PullImageOptions present the set of options available for pulling an image
// from a registry.
//
// See http://goo.gl/PhBKnS for more details.
type PullImageOptions struct {
	Repository   string `qs:"fromImage"`
	Registry     string
	OutputStream io.Writer `qs:"-"`
}

// PullImage pulls an image from a remote registry, logging progress to w.
//
// See http://goo.gl/PhBKnS for more details.
func (c *Client) PullImage(opts PullImageOptions) error {
	if opts.Repository == "" {
		return ErrNoSuchImage
	}
	return c.createImage(queryString(&opts), nil, opts.OutputStream)
}

func (c *Client) createImage(qs string, in io.Reader, w io.Writer) error {
	path := "/images/create?" + qs
	return c.stream("POST", path, in, w)
}

// ImportImageOptions present the set of informations available for importing
// an image from a source file or the stdin.
//
// See http://goo.gl/PhBKnS for more details.
type ImportImageOptions struct {
	Repository string `qs:"repo"`
	Source     string `qs:"fromSrc"`

	InputStream  io.Reader `qs:"-"`
	OutputStream io.Writer `qs:"-"`
}

// ImportImage imports an image from a url, a file or stdin
//
// See http://goo.gl/PhBKnS for more details.
func (c *Client) ImportImage(opts ImportImageOptions) error {
	if opts.Repository == "" {
		return ErrNoSuchImage
	}
	if opts.Source != "-" {
		opts.InputStream = nil
	}
	if opts.Source != "-" && !isUrl(opts.Source) {
		f, err := os.Open(opts.Source)
		if err != nil {
			return err
		}
		b, err := ioutil.ReadAll(f)
		opts.InputStream = bytes.NewBuffer(b)
		opts.Source = "-"
	}
	return c.createImage(queryString(&opts), opts.InputStream, opts.OutputStream)
}

// BuildImageOptions present the set of informations available for building
// an image from a tarball's url.
type BuildImageOptions struct {
	Name           string    `qs:"t"`
	Remote         string    `qs:"remote"`
	SuppressOutput bool      `qs:"q"`
	OutputStream   io.Writer `qs:"-"`
}

// BuildImage builds an image from a tarball's url.
func (c *Client) BuildImage(opts BuildImageOptions) error {
	if opts.Remote == "" {
		return ErrMissingRepo
	}
	// Name the image by default with the repository identifier e.g.
	// "github.com/user/repo"
	if opts.Name == "" {
		opts.Name = opts.Remote
	}
	if opts.OutputStream == nil {
		return ErrMissingOutputStream
	}
	// Call api server.
	err := c.stream("POST", fmt.Sprintf("/build?%s", queryString(&opts)), nil, opts.OutputStream)
	return err
}

func isUrl(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	return p.Scheme == "http" || p.Scheme == "https"
}

func (c *Client) GetImageTarball(imageName string, w io.Writer) error {
	return c.stream("GET", "/images/"+imageName+"/get", nil, w)
}

func (c *Client) PostImageTarball(r io.Reader) error {
	return c.stream("POST", "/images/load", r, nil)
}

type TagImageOptions struct {
	Repo  string `qs:"repo"`
	Force bool   `qs:"force"`
}

func (c *Client) SetImageTag(imageName, tag string, force bool) error {
	opts := TagImageOptions{
		Repo:  tag,
		Force: force,
	}
	path := "/images/" + imageName + "/tag?" + queryString(&opts)
	return c.stream("POST", path, nil, nil)
}
