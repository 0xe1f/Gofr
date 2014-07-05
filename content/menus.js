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

$().ready(function() {
	$(document).on('click', 'button.dropdown', function(e) {
		var topOffset = 0;
		var $button = $(this);
		var $menu = $('#' + $button.data('dropdown'));

		$('.menu').hide();
		$menu.show();

		if ($menu.hasClass('selectable')) {
			var $selected = $menu.find('.selected-menu-item');
			if ($selected.length) {
				topOffset += $selected.position().top;
				$selected.addClass('hovered').mouseout(function() {
					$selected.removeClass('hovered').unbind('mouseout');
				});
			}
		} else {
			topOffset -= $button.outerHeight() - 1; // 1 pixel for the border
		}

		$menu.css({ 'top': $button.offset().top - topOffset });
		if ($button.offset().left + $menu.width() >= $(window).width())
			$menu.css({ 'right': $(window).width() - ($button.offset().left + $button.outerWidth()) });
		else
			$menu.css({ 'left': $button.offset().left });

		e.stopPropagation();
	});

	$(document).on('click', '.menu', function(e) {
		e.stopPropagation();
	});

	$(document).on('click', '.menu li', function(e) {
		var $item = $(this);
		var $menu = $item.closest('ul');

		var groupName = null;
		$.each($item.attr('class').split(/\s+/), function() {
			if (this.indexOf('group-') == 0) {
				groupName = this;
				return false;
			}
		});

		if (groupName) {
			$('.' + groupName).removeClass('selected-menu-item');
			$item.addClass('selected-menu-item');
		}

		if ($menu.hasClass('selectable')) {
			$('.dropdown').each(function() {
				var $dropdown = $(this);
				if ($dropdown.data('dropdown') == $menu.attr('id'))
					$dropdown.text($item.text());
			});
		} else if ($item.hasClass('checkable')) {
			$item.toggleClass('checked', !$item.hasClass('checked'));
		}

		$menu.hide();

		$$menu.triggerClick($menu, $item);
	});

	$.fn.extend({
		selectItem: function(itemSelector) {
			var $menu = $(this);
			if ($menu.is('.menu.selectable')) {
				var $selected = $(itemSelector); // $menu.find(itemSelector);

				$menu.find('li').removeClass('selected-menu-item');
				$selected.addClass('selected-menu-item');

				$('.dropdown').each(function() {
					var $dropdown = $(this);
					if ($dropdown.data('dropdown') == $menu.attr('id'))
						$dropdown.text($menu.find(itemSelector).text());
				});
			}
		},
		isSelected: function(itemSelector) {
			var $menu = $(this);
			if ($menu.is('.menu.selectable'))
				return $menu.find(itemSelector).hasClass('selected-menu-item');
			return false;
		},
		setChecked: function(checked) {
			var $item = $(this);
			if ($item.is('.menu li'))
				$item.toggleClass('checked', checked);
		},
		setTitle: function(title) {
			var $item = $(this);
			if ($item.is('.menu li')) {
				$item.find('span').text(title);
				if ($item.hasClass('selected-menu-item')) {
					var $menu = $item.closest('ul');
					$('.dropdown').each(function() {
						var $dropdown = $(this);
						if ($dropdown.data('dropdown') == $menu.attr('id'))
							$dropdown.text(title);
					});
				}
			}
		},
		openMenu: function(x, y, context) {
			var $menu = $(this);
			if ($menu.hasClass('menu')) {
				$('.menu').hide();
				$menu.css( { top: y, left: x }).data('context', context).show();
			}
		}
	});
});

var $$menu = {
	clickCallback: null,
};

$$menu.click = function(callback) {
	this.clickCallback = callback;
};
$$menu.triggerClick = function($menu, $item) {
	if (this.clickCallback)
		this.clickCallback({
			'$menu': $menu,
			'$item': $item,
			'context': $menu.data('context'),
			'isChecked': $item.hasClass('checked'),
		});
};
$$menu.hideAll = function() {
	$('.menu').hide();
};
