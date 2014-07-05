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

var dateTimeFormatter = function(date, sameDay) {
	if (sameDay) {
		// Return time string (e.g. "10:30 AM")
		var hours = date.getHours();
		var ampm = (hours < 12) ? "AM" : "PM";
		var twelveHourHours = hours;

		if (hours == 0) twelveHourHours = 12;
		else if (hours > 12) twelveHourHours -= 12;

		var minutes = date.getMinutes() + "";
		if (minutes.length < 2)
			minutes = "0" + minutes;

		return twelveHourHours + ":" + minutes + " " + ampm;
	} else {
		// Return date string (e.g. "Jan 5, 2010")
		var months = [ "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec" ];
		return months[date.getMonth()] + " " + date.getDate() + ", " + date.getFullYear();
	}
};
