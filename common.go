/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/melllvar/Gofr
 ** Copyright (C) 2013 Akop Karapetyan
 **
 ** This program is free software; you can redistribute it and/or modify
 ** it under the terms of the GNU General Public License as published by
 ** the Free Software Foundation; either version 2 of the License, or
 ** (at your option) any later version.
 **
 ** This program is distributed in the hope that it will be useful,
 ** but WITHOUT ANY WARRANTY; without even the implied warranty of
 ** MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 ** GNU General Public License for more details.
 **
 ** You should have received a copy of the GNU General Public License
 ** along with this program; if not, write to the Free Software
 ** Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
 **
 ******************************************************************************
 */
 
package gofr

import (
	"appengine"
	"appengine/urlfetch"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	fetchDeadlineSeconds = 60
)

var validProperties = map[string]bool {
	"unread": true,
	"read":   true,
	"star":   true,
	"like":   true,
}

var supportedFavIconMimeTypes = []string {
	"image/vnd.microsoft.icon",
	"image/png",
	"image/gif",
}

func createHttpClient(context appengine.Context) *http.Client {
	return &http.Client {
		Transport: &urlfetch.Transport {
			Context: context,
			Deadline: time.Duration(fetchDeadlineSeconds) * time.Second,
		},
	}
}

// resolveURL accepts two URLs and returns the partialURL resolved
// in terms of the sourceURL. If partialURL is already absolute, it's
// returned as-is.
func resolveURL(sourceURL string, partialURL string) (string, error) {
	if refURL, err := url.Parse(partialURL); err != nil {
		return "", err
	} else if !refURL.IsAbs() {
		// URL is not absolute. Resolve it.
		if asURL, err := url.Parse(sourceURL); err != nil {
			return "", err
		} else {
			return asURL.ResolveReference(refURL).String(), nil
		}
	}

	return partialURL, nil
}

// extractLinks parses HTML for any link tags and returns an array
// containing the attributes of each tag as a map. Attribute keys are
// automatically converted to lowercase.
func extractLinks(html string) []map[string]string {
	tagRe := regexp.MustCompile(`<link(?:\s+\w+\s*=\s*(?:"[^"]*"|'[^']'))+\s*/?>`)
	attrRe := regexp.MustCompile(`\b(?P<key>\w+)\s*=\s*(?:"(?P<value>[^"]*)"|'(?P<value>[^'])')`)

	links := make([]map[string]string, 0, 20)
	for _, linkTag := range tagRe.FindAllString(html, -1) {
		link := make(map[string]string)

		for _, attr := range attrRe.FindAllStringSubmatch(linkTag, -1) {
			key := strings.ToLower(attr[1])
			if attr[2] != "" {
				link[key] = strings.ToLower(attr[2])
			} else if attr[3] != "" {
				link[key] = strings.ToLower(attr[3])
			}
		}

		links = append(links, link)
	}

	return links
}

// locateFavIconURL attempts to determine the "favicon" URL for a particular
// site URL. It does this by checking the source document for explicit icon
// directives (in the LINK tags), as well as by attempting to fetch favicon.ico
func locateFavIconURL(context appengine.Context, feedHomeURL string) (string, error) {
	if feedHomeURL != "" {
		// Attempt to extract the favicon from the source document
		if favIconURL, err := extractFavIconURL(context, feedHomeURL); err != nil {
			context.Warningf("FavIcon extraction failed for %s: %s", feedHomeURL, err)
		} else if favIconURL != "" {
			if contains, err := containsFavIcon(context, favIconURL); err != nil {
				context.Warningf("FavIcon lookup failed for %s: %s", feedHomeURL, err)
			} else if contains {
				return favIconURL, nil
			}
		}

		// If that fails, try the usual location (/favicon.ico)
		if url, err := url.Parse(feedHomeURL); err != nil {
			return "", err
		} else {
			attemptURL := fmt.Sprintf("%s://%s/favicon.ico", url.Scheme, url.Host)
			if contains, err := containsFavIcon(context, attemptURL); err != nil {
				return "", err
			} else if contains {
				return attemptURL, nil
			}
		}
	}

	return "", nil
}

// containsFavIcon return true if a URL contains a valid "favicon".
// A valid favicon has one of the supported MIME types.
func containsFavIcon(c appengine.Context, favIconURL string) (bool, error) {
	client := createHttpClient(c)
	if response, err := client.Get(favIconURL); err == nil {
		defer response.Body.Close()

		bytes, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return false, err
		}

		mimeType := http.DetectContentType(bytes)
		for _, acceptedMimeType := range supportedFavIconMimeTypes {
			if mimeType == acceptedMimeType {
				return true, nil
			}
		}
	} else {
		return false, err
	}

	return false, nil
}

// extractFavIconURL parses an HTML document and extracts an explicit
// "favicon" URL specified by the LINK tag. If an icon is found
// a call to containsFavIcon is made to make sure the URL is actually
// valid.
func extractFavIconURL(context appengine.Context, sourceURL string) (string, error) {
	client := createHttpClient(context)
	if response, err := client.Get(sourceURL); err != nil {
		return "", err
	} else {
		defer response.Body.Close()

		if contents, err := ioutil.ReadAll(response.Body); err != nil {
			return "", err
		} else {
			html := string(contents)
			links := extractLinks(html)

			for _, link := range links {
				if (link["rel"] == "icon" || link["rel"] == "shortcut icon") && link["href"] != "" {
					return resolveURL(sourceURL, link["href"])
				}
			}
		}
	}

	return "", nil
}
