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
	"encoding/xml"
	"strings"
	"time"
)

var supportedAtomTimeFormats = []string {
	"2006-01-02T15:04:05Z07:00",
	"January 2, 2006",
}

type atomFeed struct {
	XMLName xml.Name `xml:"feed"`
	Id string `xml:"id"`
	Title string `xml:"title"`
	Description string `xml:"subtitle"`
	Updated string `xml:"updated"`
	Link []atomLink `xml:"link"`
	Entry []*atomEntry `xml:"entry"`
}

type atomLink struct {
	Type string `xml:"type,attr"`
	Rel string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
	URI string `xml:"uri"`
}

type atomEntry struct {
	Id string `xml:"id"`
	Published string `xml:"published"`
	Updated string `xml:"updated"`
	Link []atomLink `xml:"link"`
	EntryTitle atomText `xml:"title"`
	Content atomText `xml:"content"`
	Summary atomText `xml:"summary"`
	Author atomAuthor `xml:"author"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

func (nativeFeed *atomFeed) Marshal() (feed Feed, err error) {
	updated := time.Time {}
	if nativeFeed.Updated != "" {
		updated, err = parseTime(supportedAtomTimeFormats, nativeFeed.Updated)
	}

	hubURL := ""
	linkUrl := ""
	topic := ""

	for _, link := range nativeFeed.Link {
		rels := strings.Split(link.Rel, " ")
		for _, rel := range rels {
			if rel == "alternate" {
				linkUrl = link.Href
				break
			} else if rel == "self" {
				topic = link.Href
				break
			} else if rel == "hub" {
				hubURL = link.Href
				break
			}
		}
	}

	feed = Feed {
		Title: nativeFeed.Title,
		Description: nativeFeed.Description,
		Updated: updated,
		WWWURL: linkUrl,
		Format: "Atom",
		HubURL: hubURL,
		Topic: topic,
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

func (nativeEntry *atomEntry) Marshal() (entry *Entry, err error) {
	linkUrl := ""
	for _, link := range nativeEntry.Link {
		if linkUrl == "" || link.Rel == "alternate" {
			linkUrl = link.Href
		}
	}

	guid := nativeEntry.Id
	
	content := nativeEntry.Content.Content
	if content == "" && nativeEntry.Summary.Content != "" {
		content = nativeEntry.Summary.Content
	}

	published := time.Time {}
	if nativeEntry.Published != "" {
		published, err = parseTime(supportedAtomTimeFormats, nativeEntry.Published)
	}

	updated := published
	if nativeEntry.Updated != "" {
		updated, err = parseTime(supportedAtomTimeFormats, nativeEntry.Updated)
		if published.IsZero() {
			published = updated // e.g. xkcd
		}
	}

	entry = &Entry {
		GUID: guid,
		Author: nativeEntry.Author.Name,
		Title: nativeEntry.EntryTitle.Content,
		Content: content,
		Published: published,
		Updated: updated,
		WWWURL: linkUrl,
	}

	return entry, err
}
