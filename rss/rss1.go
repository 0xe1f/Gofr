/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
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
	"time"
	"encoding/xml"
)

var supportedRSS1TimeFormats = []string {
	"2006-01-02T15:04-07:00",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02",
}

type rssLink struct {
	XMLName xml.Name `xml:"link"`
	Content string `xml:",chardata"`
	Type string `xml:"type,attr"`
	Rel string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type rss1Feed struct {
	XMLName xml.Name `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# RDF"`
	Title string `xml:"channel>title"`
	Description string `xml:"channel>description"`
	Updated string `xml:"channel>date"`
	Link []*rssLink `xml:"channel>link"`
	Entry []*rss1Entry `xml:"item"`
}

type rss1Entry struct {
	Id string `xml:"guid"`
	Published string `xml:"http://purl.org/dc/elements/1.1/ date"`
	EntryTitle string `xml:"title"`
	Link string `xml:"link"`
	Author string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	EncodedContent string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Content string `xml:"description"`
}

func (nativeFeed *rss1Feed) Marshal() (feed Feed, err error) {
	updated := time.Time {}
	if nativeFeed.Updated != "" {
		updated, err = parseTime(supportedRSS1TimeFormats, nativeFeed.Updated)
	}

	linkUrl := ""
	for _, linkNode := range nativeFeed.Link {
		if linkNode.XMLName.Space == "http://purl.org/rss/1.0/" {
			linkUrl = linkNode.Content
		}
	}

	feed = Feed {
		Title: nativeFeed.Title,
		Description: nativeFeed.Description,
		Updated: updated,
		WWWURL: linkUrl,
		Format: "RSS1",
	}

	if nativeFeed.Entry != nil {
		feed.Entries = make([]*Entry, len(nativeFeed.Entry))
		for i, v := range nativeFeed.Entry {
			var entryError error
			feed.Entries[i], entryError = v.Marshal()

			if entryError != nil && err == nil {
				err = entryError
			}
		}
	}

	return feed, err
}

func (nativeEntry *rss1Entry) Marshal() (entry *Entry, err error) {
	guid := nativeEntry.Id
	content := nativeEntry.EncodedContent
	if content == "" {
		content = nativeEntry.Content
	}

	published := time.Time {}
	if nativeEntry.Published != "" {
		published, err = parseTime(supportedRSS1TimeFormats, nativeEntry.Published)
	}

	entry = &Entry {
		GUID: guid,
		Author: nativeEntry.Author,
		Title: nativeEntry.EntryTitle,
		Content: content,
		Published: published,
		WWWURL: nativeEntry.Link,
	}

	return entry, err
}
