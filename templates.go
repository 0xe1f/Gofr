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

const introTemplateHTML = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
  <head profile="http://www.w3.org/2005/10/profile">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
    <link href="/content/intro.css" type="text/css" rel="stylesheet"/>
    <title>Gofr</title>
  </head>
  <body>
    <div class="content">
      <div class="header">
        <button class="sign-in" onclick="window.location='/reader';">Sign in with Google</button>
        <div class="clear"></div>
      </div>
      <div class="stripe">
        <div class="text">
          <img src="/content/logo.png" alt="Logo" />
          <h1>Gofr</h1>
          <h3>A cloud-based RSS reader that's free and open source.</h3>
        </div>
      </div>
      <div class="features">
        <div class="boxen">
          <div class="box">
            <h3>Flexible.</h3>
            <p>Navigate by using mouse or keyboard, with support for most of Google Reader's shortcut keys.</p>
          </div>
          <div class="box">
            <h3>Portable.</h3>
            <p>Subscribe to individual feeds, or import all your existing subscriptions - up to 400.</p>
          </div>
          <div class="box">
            <h3>Extensible.</h3>
            <p>Gofr is open source, and available on <a href="https://github.com/melllvar/Gofr">GitHub</a> - use it as-is, or improve it and share with everyone else.</p>
          </div>
        </div>
        <div class="hr"></div>
        <div class="bottom">
          <div class="box">
            <img src="https://developers.google.com/appengine/images/appengine-noborder-120x30.gif" alt="Powered by Google App Engine" />
            <h3>Runs on <a href="https://developers.google.com/appengine/">Google App Engine</a></h3>
          </div>
          <div class="box">
            <img src="content/logo-golang.png" alt="Written in Go" />
            <h3>Written in <a href="http://www.golang.org/">Go</a> and <a href="http://jquery.com/">jQuery</a></h3>
          </div>
          <img class="screenshot" src="/content/screenshot.png" alt="Screenshot" />
        </div>
      </div>
      <div class="footer">
        &copy; 2013 <a href="http://www.akop.org/">Akop Karapetyan</a>
        &bull; <a class="license" href="https://raw.github.com/melllvar/Gofr/master/LICENSE">License</a>
        &bull; <a class="source" href="https://github.com/melllvar/Gofr">Source</a>
      </div>
    </div>
  </body>
</html>
`
const readerTemplateHTML = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
  <head profile="http://www.w3.org/2005/10/profile">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
    <link href="content/reader.css" type="text/css" rel="stylesheet"/>
    <link href="content/mobile.css" type="text/css" rel="stylesheet"/>
    <script type="text/javascript" src="/_ah/channel/jsapi"></script>
    <script src="content/sprintf.min.js" type="text/javascript"></script>
    <script src="content/jquery-1.9.1.min.js" type="text/javascript"></script>
    <script src="content/jquery.hotkeys.js" type="text/javascript"></script>
    <script src="content/jquery.cookie.js" type="text/javascript"></script>
    <script src="content/jquery.form.min.js" type="text/javascript"></script>
    <script src="content/jquery.scrollintoview.min.js" type="text/javascript"></script>
    <script src="content/l10n/default.js" type="text/javascript"></script>
    <script src="content/l10n/en-us.js" type="text/javascript"></script>
    <script src="content/menus.js" type="text/javascript"></script>
    <script src="content/reader.js" type="text/javascript"></script>
    <title>Gofr</title>
  </head>
  <body>
    <div id="toast"><span></span></div>
    <div id="header">
      <h1>Gofr</h1>
      <div class="infobar">
        <div class="right-aligned">
          <button class="user-options dropdown" data-dropdown="menu-user-options" title="{{.UserEmail}}">{{.UserEmail}}</button>
          <a id="sign-out" href="{{.LogOutURL}}">sign out</a>
        </div>
      </div>
    </div>
    <div class="navbar">
      <div class="right-aligned">
        <button class="settings dropdown _l" data-dropdown="menu-settings" title="Settings">&nbsp;</button>
        <button class="select-article up _l" title="Previous Article">&nbsp;</button><button class="select-article down _l" title="Next Article">&nbsp;</button>
      </div>
      <button class="navigate _l">&nbsp;</button>
      <button class="refresh _l" title="Refresh">&nbsp;</button>
      <button class="filter dropdown _l" data-dropdown="menu-filter">All Items</button>
      <button class="mark-all-as-read _l">Mark all as read</button>
    </div>
    <div id="reader">
      <div class="feeds-container">
        <button class="subscribe solid-color _l">Subscribe</button>
        <ul id="subscriptions"></ul>
      </div>
      <div class="gofr-entries-container">
        <div class="center-message"></div>
        <div class="gofr-entries-header"></div>
        <div id="gofr-entries"></div>
      </div>
    </div>
    <div id="footer">
      <a class="about _l" href="http://www.akop.org/">About</a>
      &bull; <a class="license _l" href="https://raw.github.com/melllvar/Gofr/master/LICENSE">License</a>
      &bull; <a class="source _l" href="https://github.com/melllvar/Gofr">Source</a>
    </div>
    <div id="floating-nav"></div>
    <div class="modal-blocker"></div>
    <div id="import-subscriptions" class="modal">
      <h1 class="_l">Upload OPML file</h1>
      <form enctype="multipart/form-data" action="#" method="post">
        <div>
          <input name="opml" type="file" />
          <input name="client" type="hidden" value="" />
        </div>
      </form>
      <div class="buttons">
        <button class="modal-cancel _l">Cancel</button>
        <button class="modal-ok _l">Upload</button>
      </div>
    </div>
    <div id="about" class="modal">
      <p><b>Gofr</b> is an open source Feed Reader 
      (Google Reader clone) for 
      <a href="https://developers.google.com/appengine/">Google App Engine</a>, 
      with source code available on 
      <a href="https://github.com/melllvar/Gofr">GitHub</a>.</p>
      <p>It's written in <a href="http://golang.org/">Go</a> and JavaScript 
      (using <a href="http://jquery.com/">jQuery</a>) and is loosely based on 
      <a href="https://github.com/melllvar/grr">grr</a> - 
      an initial implementation written for PHP/MySQL.</p>
      <p>Gofr is written by <a href="http://www.akop.org/">Akop Karapetyan</a>.</p>
      <button class="modal-cancel _l">Close</button>
    </div>
  </body>
</html>
`
