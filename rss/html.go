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
  "strings"
  "regexp"
)

func ExtractRSSLink(html string) string {
  tagRe := regexp.MustCompile(`<link(?:\s+\w+\s*=\s*(?:"[^"]*"|'[^']'))+\s*/?>`)
  attrRe := regexp.MustCompile(`\b(?P<key>\w+)\s*=\s*(?:"(?P<value>[^"]*)"|'(?P<value>[^'])')`)

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

    if link["rel"] == "alternate" && link["type"] == "application/rss+xml" {
      return link["href"]
    }
  }

  return ""
}
