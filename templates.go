/*****************************************************************************
 **
 ** FRAE
 ** https://github.com/melllvar/frae
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
 
package frae

import (
  "html/template"
)

var indexTemplate = template.Must(template.New("index").Parse(indexTemplateHTML))

const indexTemplateHTML = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
  <head profile="http://www.w3.org/2005/10/profile">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
    <link href="content/reader.css" type="text/css" rel="stylesheet"/>
    <script src="content/sprintf.min.js" type="text/javascript"></script>
    <script src="content/jquery-1.9.1.min.js" type="text/javascript"></script>
    <script src="content/jquery.scrollintoview.min.js" type="text/javascript"></script>
    <script src="content/reader.js" type="text/javascript"></script>
    <title>&gt;:(</title>
  </head>
  <body>
    <div id="toast"><span></span></div>
    <div id="header">
      Houmu rogo, desu ne?
    </div>
    <div id="navbar">
      <div class="right-aligned">
        <button class="select-article up" title="Previous Article"></button><button class="select-article down" title="Next Article"></button>
      </div>
      <button class="navigate">Navigate</button>
      <button class="refresh" title="Refresh">&nbsp;</button>
      <button class="filter dropdown">All Items</button>
      <button class="mark-all-as-read">Mark all as read</button>
    </div>
    <div id="reader">
      <div class="feeds-container">
        <button class="subscribe">Subscribe</button>
        <ul id="subscriptions"></ul>
      </div>
      <div class="entries-container">
        <div class="center-message"></div>
        <div class="entries-header"></div>
        <div id="entries"></div>
      </div>
    </div>
    <div id="floating-nav"></div>
  </body>
</html>
`
