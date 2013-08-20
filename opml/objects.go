/*****************************************************************************
 **
 ** PerFeediem
 ** https://github.com/melllvar/PerFeediem
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
 
package opml

import (
  "encoding/json"
)

type opmlRoot struct {
  Body opmlBody `xml:"body"`
}

type opmlBody struct {
  Subscriptions []*Subscription `xml:"outline"`
}

type Subscription struct {
  Title string `xml:"title,attr"`
  URL string `xml:"xmlUrl,attr"`
  Subscriptions []*Subscription `xml:"outline"`
}

type Document struct {
  Subscriptions []*Subscription
}

func (doc Document) String() string {
  bf, _ := json.Marshal(doc)
  return string(bf)
}
