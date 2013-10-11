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
 
package rss

import (
	"appengine"
	"strings"
	"regexp"
	"net/url"
)

func ExtractRSSLink(c appengine.Context, sourceURL string, html string) (string, error) {
	tagRe := regexp.MustCompile(`<link(?:\s+\w+\s*=\s*(?:"[^"]*"|'[^']'))+\s*/?>`)
	attrRe := regexp.MustCompile(`\b(?P<key>\w+)\s*=\s*(?:"(?P<value>[^"]*)"|'(?P<value>[^'])')`)

	linkURL := ""
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

		if link["rel"] == "alternate" {
			if link["type"] == "application/rss+xml" || link["type"] == "application/atom+xml" {
				linkURL = link["href"]
			}
		}
	}

	if linkURL != "" {
		if refURL, err := url.Parse(linkURL); err != nil {
			return "", err
		} else if !refURL.IsAbs() {
			// URL is not absolute. Resolve it.
			if asURL, err := url.Parse(sourceURL); err != nil {
				return "", err
			} else {
				linkURL = asURL.ResolveReference(refURL).String()
			}
		}
	}

	return linkURL, nil
}
