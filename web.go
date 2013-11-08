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
	"appengine/user"
	"encoding/xml"
	"html/template"
	"net/http"
	"storage"
)

func registerWeb() {
	RegisterHTMLRoute("/reader", reader)
	RegisterHTMLRoute("/export",  exportOPML)

	RegisterAnonHTMLRoute("/",    intro)
}

var readerTemplate = template.Must(template.New("reader").Parse(readerTemplateHTML))
var introTemplate = template.Must(template.New("intro").Parse(introTemplateHTML))

func intro(pfc *PFContext) {
	if pfc.User != nil {
		pfc.W.Header().Set("Location", "/reader")
		pfc.W.WriteHeader(http.StatusFound)
		return
	}

	if err := introTemplate.Execute(pfc.W, nil); err != nil {
		http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
	}
}

func reader(pfc *PFContext) {
	content := map[string]string {
		"UserEmail": pfc.User.EmailAddress,
	}
	if logoutURL, err := user.LogoutURL(pfc.C, "/"); err == nil {
		content["LogOutURL"] = logoutURL
	}

	if err := readerTemplate.Execute(pfc.W, content); err != nil {
		http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
	}
}

func exportOPML(pfc *PFContext) {
	c := pfc.C
	w := pfc.W

	if opml, err := storage.SubscriptionsAsOPML(c, pfc.UserID); err != nil {
		c.Errorf("Error retrieving list of subscriptions: %s", err)
		http.Error(w, _l("Error retrieving list of subscriptions"), http.StatusInternalServerError)
		return
	} else {
		opml.SetTitle(_l("Gofr subscriptions for %s", pfc.User.EmailAddress))

		if output, err := xml.MarshalIndent(opml, "", "    "); err != nil {
			c.Errorf("Error generating XML: %s", err)
			http.Error(w, _l("Error generating subscriptions"), http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-disposition", "attachment; filename=subscriptions.xml");
			w.Header().Set("Content-type", "application/xml; charset=utf-8")

			w.Write([]byte(xml.Header))
			w.Write(output)
		}
	}
}
