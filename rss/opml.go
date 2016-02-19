/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
 ** Copyright (C) 2013-2016 Akop Karapetyan
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
	"io"
)

type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string `xml:"version,attr,omitempty"`
	Head head `xml:"head"`
	Body opmlBody `xml:"body"`
}

type head struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []*Outline `xml:"outline"`
}

type Outline struct {
	Text string `xml:"text,attr"`
	Title string `xml:"title,attr"`
	Type string `xml:"type,attr,omitempty"`
	FeedURL string `xml:"xmlUrl,attr,omitempty"`
	WebURL string `xml:"htmlUrl,attr"`
	Outlines []*Outline `xml:"outline"`
}

func (opml *OPML)Title() string {
	return opml.Head.Title
}

func (opml *OPML)SetTitle(title string) {
	opml.Head.Title = title
}

func (opml *OPML)Outlines() []*Outline {
	return opml.Body.Outlines
}

func (outline Outline)IsFolder() bool {
	return outline.FeedURL == ""
}

func (outline Outline)IsSubscription() bool {
	return outline.FeedURL != ""
}

func NewOPML() OPML {
	return OPML {
		Version: "1.0",
	}
}

func NewFolder(title string) *Outline {
	return &Outline {
		Text: title,
		Title: title,
	}
}

func NewSubscription(title string, feedURL string, webURL string) *Outline {
	return &Outline {
		Text: title,
		Title: title,
		FeedURL: feedURL,
		WebURL: webURL,
	}
}

func (opml *OPML)Add(outline *Outline) {
	opml.Body.Outlines = append(opml.Body.Outlines, outline)
}

func (outline *Outline)Add(child *Outline) {
	outline.Outlines = append(outline.Outlines, child)
}

func ParseOPML(reader io.Reader) (*OPML, error) {
	var opml OPML
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(&opml); err != nil {
		return nil, err
	}

	return &opml, nil
}
