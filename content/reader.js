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

$().ready(function() {
  var $dragSource = null;
  var dragDestination = null;

  var clientId = null;
  var subscriptionMap = null;
  var continueFrom = null;
  var lastContinued = null;
  var lastGPressTime = 0;
  var channel;

  var _l = function(str, args) {
    var localized = null;
    if (typeof gofrStrings !== 'undefined' && gofrStrings != null)
      localized = gofrStrings[str];

    if (localized == null)
      localized = str; // No localization

    if (args)
      return vsprintf(localized, args);

    return localized;
  };

  var getPublishedDate = function(dateAsString) {
    var now = new Date();
    var date = new Date(dateAsString);
    
    var sameDay = now.getDate() == date.getDate() 
      && now.getMonth() == date.getMonth() 
      && now.getFullYear() == date.getFullYear();

    return dateTimeFormatter(date, sameDay);
  };

  // Automatic pager

  $('.entries-container').scroll(function() {
    var pagerHeight = $('.next-page').outerHeight();
    if (!pagerHeight)
      return; // No pager

    if (lastContinued == continueFrom)
      return;

    var offset = $('#entries').height() - ($('.entries-container').scrollTop() + $('.entries-container').height()) - pagerHeight;
    if (offset < 36)
      $('.next-page').click();
  });

  // Default click handler

  $('html')
    .click(function() {
      $('.shortcuts').hide();
      $('#floating-nav').hide();
      $$menu.hideAll();
    })
    .mouseup(function(e) {
      $('#subscriptions').unbind("mousemove");
      $('#subscriptions .dragging').remove();
      $('#subscriptions .dragged').removeClass('dragged');

      if ($dragSource && dragDestination) {
        var dragSource = $dragSource.data('subscription');
        if (dragDestination.id != (dragSource.parent || ''))
          dragSource.moveTo(dragDestination);
      }

      $dragSource = null;
      dragDestination = null;
    });

  $('.modal-blocker').click(function() {
    $('.modal').showModal(false);
  });

  // Default error handler

  $(document).ajaxError(function(event, jqxhr, settings, exception) {
    var errorMessage;

    try {
      var errorJson = $.parseJSON(jqxhr.responseText)
      errorMessage = errorJson.errorMessage;
    } catch (exception) {
      errorMessage = _l("An unexpected error has occurred. Please try again later.");
    }

    if (errorMessage != null)
      ui.showToast(errorMessage, true);
    else if (errorJson.infoMessage != null)
      ui.showToast(errorJson.infoMessage, false);
  });

  var subscriptionMethods = {
    'getDom': function() {
      return $('#subscriptions').find('.' + this.domId);
    },
    'isFolder': function() {
      return false;
    },
    'isRoot': function() {
      return false;
    },
    'getChildren': function() {
      var subscription = this;
      var children = [];

      $.each(subscriptionMap, function(key, sub) {
        if (subscription.id === "" || sub.parent === subscription.id)
          children.push(sub);
      });

      return children;
    },
    'addPage': function(entries) {
      var subscription = this;
      var idCounter = $('#entries').find('.entry').length;

      $.each(entries, function() {
        var entry = this;
        var details = entry.details;

        // Inject methods
        for (var name in entryMethods)
          entry[name] = entryMethods[name];

        var entrySubscription = entry.getSubscription();
        if (!entrySubscription)
          return true; // May have been deleted on server; don't add it if so

        entry.domId = 'entry-' + idCounter++;
        var $entry = $('<div />', { 'class' : 'entry ' + entry.domId})
          .data('entry', entry)
          .append($('<div />', { 'class' : 'entry-item' })
            .append($('<div />', { 'class' : 'action-star' })
              .click(function(e) {
                entry.toggleStarred();
                e.stopPropagation();
              }))
            .append($('<span />', { 'class' : 'entry-source' })
              .text(entrySubscription != null ? entrySubscription.title : null))
            .append($('<a />', { 'class' : 'entry-link', 'href' : details.link, 'target' : '_blank' })
              .click(function(e) {
                e.stopPropagation();
              }))
            .append($('<span />', { 'class' : 'entry-pubDate' })
              .text(getPublishedDate(details.published)))
            .append($('<div />', { 'class' : 'entry-excerpt' })
              .append($('<h2 />', { 'class' : 'entry-title' })
                .text(details.title))))
          .click(function() {
            entry.select();
            
            var wasExpanded = entry.isExpanded();

            ui.collapseAllEntries();
            if (!wasExpanded) {
              entry.expand();
              entry.scrollIntoView();
            }
          });

        if (details.summary) {
          $entry.find('.entry-excerpt')
            .append($('<span />', { 'class' : 'entry-spacer' }).text(' - '))
            .append($('<span />', { 'class' : 'entry-summary' }).text(details.summary));
        }

        $('#entries').append($entry);

        entry.syncView();
      });

      $('.next-page').remove();

      ui.onEntryListUpdate();

      if (continueFrom) {
        $('#entries')
          .append($('<div />', { 'class' : 'next-page' })
            .text(_l('Continue'))
            .click(function(e) {
              subscription.loadEntries();
            }));
      }
    },
    'loadEntries': function() {
      lastContinued = continueFrom;

      var subscription = this;
      var selectedFilter = $('.group-filter.selected-menu-item').data('value');

      $.getJSON('articles', {
        'subscription': !subscription.isFolder() ? subscription.id : undefined,
        'folder':       subscription.isFolder() ? subscription.id : subscription.parent,
        'continue':     continueFrom ? continueFrom : undefined,
        'filter':       selectedFilter,
        'client':       clientId,
      })
      .success(function(response) {
        continueFrom = response.continue;
        subscription.addPage(response.articles, response.continue);
      });
    },
    'refresh': function() {
      continueFrom = null;
      lastContinued = null;

      $('#entries').empty();
      this.loadEntries();
    },
    'select': function(reloadItems /* = true */) {
      if (typeof reloadItems === 'undefined')
        reloadItems = true;

      $('#subscriptions').find('.subscription.selected').removeClass('selected');
      this.getDom().addClass('selected');

      if (reloadItems) {
        this.selectedEntry = null;

        $('#entries').toggleClass('single-source', this.link != null);

        if (!this.link)
          $('.entries-header').text(this.title);
        else
          $('.entries-header').html($('<a />', { 'href' : this.link, 'target' : '_blank' })
            .text(this.title)
            .append($('<span />')
              .text(' »')));

        this.refresh();
        ui.updateUnreadCount();
      }
    },
    'syncView': function() {
      var $feed = this.getDom();
      var $item = $feed.find('> .subscription-item');

      $item.find('.subscription-title').text(this.title);
      $item.find('.subscription-unread-count').text('(' + this.unread + ')');
      $item.toggleClass('has-unread', this.unread > 0);
      $feed.toggleClass('no-unread', this.unread < 1);

      var parent = this.getParent();
      if (parent)
        parent.syncView();

      if (!this.isRoot())
        this.getRoot().syncView();
    },
    'getType': function() {
      return 'leaf';
    },
    'getParent': function() {
      if (!this.parent)
        return null;

      return subscriptionMap[this.parent];
    },
    'getRoot': function() {
      return subscriptionMap[''];
    },
    'updateUnreadCount': function(byHowMuch) {
      this.unread += byHowMuch;

      var parent = this.getParent();
      if (parent != null)
        parent.unread += byHowMuch;

      this.getRoot().unread += byHowMuch;
    },
    'rename': function(newName) {
      var subscription = this;

      $.post('rename', {
        'subscription': subscription.isFolder() ? undefined : subscription.id,
        'folder': subscription.isFolder() ? subscription.id : subscription.parent,
        'title': newName,
      }, 
      function(response) {
        resetSubscriptionDom(response, false);
      }, 'json');
    },
    'unsubscribe': function() {
      var subscription = this;
      if (!subscription.isFolder()) {
        $.post('unsubscribe', {
          'client': clientId,
          'subscription': subscription.id,
          'folder': subscription.parent,
        }, 
        function(response) {
          resetSubscriptionDom(response, false);
          $('#entries .entry').each(function() {
            var $entry = $(this);
            var entry = $entry.data('entry');

            if (entry.source == subscription.id)
              $entry.remove();

            ui.onEntryListUpdate();
          });
        }, 'json');
      }
    },
    'markAllAsRead': function(filter) {
      var subscription = this;

      $.post('markAllAsRead', {
        'client':       clientId,
        'subscription': subscription.isFolder() ? undefined : subscription.id,
        'folder':       subscription.isFolder() ? subscription.id : subscription.parent,
        'filter':       filter,
      },
      function(response) {
        ui.showToast(response.message);
        if (response.done)
          refresh();
      }, 'json');
    },
    'moveTo': function(folder) {
      var subscription = this;

      $.post('moveSubscription', {
        'client':       clientId,
        'subscription': subscription.id,
        'folder':       subscription.parent ? subscription.parent : undefined,
        'destination':  folder.id ? folder.id : undefined,
      },
      function(response) {
        ui.showToast(response.message);
        if (response.done)
          refresh();
      }, 'json');
    },
    'removeFolder': function() {
      var subscription = this;
      if (subscription.isFolder()) {
        $.post('removeFolder', {
          'client': clientId,
          'folder': subscription.id,
        }, 
        function(response) {
          ui.showToast(response.message);
        }, 'json');
      }
    },
  };

  var folderMethods = $.extend({}, subscriptionMethods, {
    'subscribe': function(url) {
      var params = {
        'url': url,
        'client': clientId,
      };
      if (this.id)
        params['folder'] = this.id;

      $.post('subscribe', params, function(response) {
        resetSubscriptionDom(response, false);
      }, 'json');
    },
    'isFolder': function() {
      return true;
    },
    'isRoot': function() {
      return this.id == "";
    },
    'getType': function() {
      if (this.isRoot())
        return 'root';

      return 'folder';
    },
  });

  var entryMethods = {
    'getSubscription': function() {
      return subscriptionMap[this.source];
    },
    'getDom': function() {
      return $('#entries').find('.' + this.domId);
    },
    'hasProperty': function(propertyName) {
      return $.inArray(propertyName, this.properties) > -1;
    },
    'setProperty': function(propertyName, propertyValue) {
      if (propertyValue == this.hasProperty(propertyName))
        return; // Already set

      var entry = this;

      $.post('setProperty', {
        'article':      this.id,
        'subscription': this.source,
        'folder':       this.getSubscription().parent,
        'property':     propertyName,
        'set':          propertyValue,
      },
      function(properties) {
        delete entry.properties;

        entry.properties = properties;
        entry.syncView();

        if (propertyName == 'read') {
          var subscription = entry.getSubscription();
          subscription.updateUnreadCount(propertyValue ? -1 : 1);

          subscription.syncView();
          ui.updateUnreadCount();
        }
      }, 'json');
    },
    'toggleStarred': function(propertyName) {
      this.toggleProperty("star");
    },
    'toggleUnread': function() {
      this.toggleProperty("read");
    },
    'toggleProperty': function(propertyName) {
      this.setProperty(propertyName, 
        !this.hasProperty(propertyName));
    },
    'syncView': function() {
      this.getDom()
        .toggleClass('star', this.hasProperty('star'))
        .toggleClass('like', this.hasProperty('like'))
        .toggleClass('read', this.hasProperty('read'));
    },
    'isExpanded': function() {
      return this.getDom().hasClass('open');
    },
    'expand': function() {
      var entry = this;
      var details = entry.details;
      var subscription = this.getSubscription();
      var $entry = this.getDom();

      if (this.isExpanded())
        return;

      if (!this.hasProperty('read'))
        this.setProperty('read', true);

      var content = 
        $('<div />', { 'class' : 'entry-content' })
          .append($('<div />', { 'class' : 'article' })
            .append($('<a />', { 'href' : details.link, 'target' : '_blank', 'class' : 'article-title' })
              .append($('<h2 />')
                .text(details.title)))
            .append($('<div />', { 'class' : 'article-author' })
              .append('from ')
              .append($('<a />', { 'href' : subscription.link, 'target' : '_blank' })
                .text(subscription.title)))
            .append($('<div />', { 'class' : 'article-body' })
              .append(details.content)))
          .append($('<div />', { 'class' : 'entry-footer'})
            .append($('<span />', { 'class' : 'action-star' })
              .click(function(e) {
                entry.toggleStarred();
              }))
            .append($('<span />', { 'class' : 'action-unread entry-action'})
              .text(_l('Keep unread'))
              .click(function(e) {
                entry.toggleUnread();
              }))
          )
          .click(function(e) {
            e.stopPropagation();
          });

      if (details.author)
        content.find('.article-author')
          .append(' by ') // FIXME: localize
          .append($('<span />')
            .text(details.author));

      // Links in the content should open in a new window
      content.find('.article-body a').attr('target', '_blank');

      $entry.toggleClass('open', true);
      $entry.append(content);
    },
    'scrollIntoView': function() {
      this.getDom().scrollintoview({ duration: 0});
    },
    'collapse': function() {
      this.getDom()
        .removeClass('open')
        .find('.entry-content')
          .remove();
    },
    'select': function() {
      $('#entries').find('.entry.selected').removeClass('selected');
      this.getDom().addClass('selected');
    },
  };

  $$menu.click(function(e) {
    var $item = e.$item;
    if ($item.is('.menu-all-items, .menu-new-items, .menu-starred-items')) {
      var subscription = getSelectedSubscription();
      if (subscription != null)
        subscription.refresh();
      onFilterChanged();
    } else if ($item.is('.menu-show-sidebar')) {
      ui.toggleSidebar(e.isChecked);
    } else if ($item.is('.menu-show-all-subs')) {
      ui.toggleReadSubscriptions(e.isChecked);
    } else if ($item.is('.menu-create-folder')) {
      ui.createFolder();
    } else if ($item.is('.menu-subscribe, .menu-rename, .menu-unsubscribe, .menu-delete')) {
      var subscription = subscriptionMap[e.context];
      if ($item.is('.menu-subscribe')) {
        ui.subscribe(subscription);
      } else if ($item.is('.menu-rename')) {
        ui.rename(subscription);
      } else if ($item.is('.menu-unsubscribe')) {
        ui.unsubscribe(subscription);
      } else if ($item.is('.menu-delete')) {
        ui.removeFolder(subscription);
      }
    }
  });

  var onFilterChanged = function() {
    $('.mark-all-as-read').toggle(!$('#menu-filter').isSelected('.menu-starred-items'));
  };

  var ui = {
    'init': function() {
      this.initHelp();
      this.initButtons();
      this.initMenus();
      this.initShortcuts();
      this.initModals();

      $('a.import-subscriptions').click(function() {
        ui.showImportSubscriptionsModal();
        return false;
      });

      $('a.show-about').click(function() {
        ui.showAbout();
        return false;
      });

      $('#menu-filter').selectItem('.menu-all-items');

      this.toggleSidebar($.cookie('show-sidebar') !== 'false');
      this.toggleReadSubscriptions($.cookie('show-all-subs') !== 'false');
    },
    'initButtons': function() {
      $('button.refresh').click(function() {
        refresh();
      });
      $('button.subscribe').click(function() {
        ui.subscribe();
      });
      $('button.navigate').click(function(e) {
        $('#floating-nav')
          .css( { top: $('button.navigate').offset().top, left: $('button.navigate').offset().left })
          .show();

        e.stopPropagation();
      });
      $('.select-article.up').click(function() {
        ui.openArticle(-1);
      });
      $('.select-article.down').click(function() {
        ui.openArticle(1);
      });
      $('.mark-all-as-read').click(function() {
        ui.markAllAsRead();
      });

      $('#import-subscriptions .modal-ok').click(function() {
        var $modal = $(this).closest('.modal');
        var $form = $('#import-subscriptions form');

        if (!$form.find('input[type=file]').val()) {
          // No file specified
          return;
        }

        // Get upload URL
        $.post('authUpload', {
        },
        function(response) {
          // Set the client ID
          $form.find('input[name=client]').val(clientId);

          // Upload the file
          $form
            .attr('action', response.uploadUrl)
            .ajaxSubmit( {
              success: function(response) {
                ui.showToast(response.message, false);
              },
              dataType: 'json',
            });

          // Dismiss the modal
          $modal.showModal(false);
        }, 'json');
      });
    },
    'initMenus': function() {
      $('body')
        .append($('<ul />', { 'id': 'menu-filter', 'class': 'menu selectable' })
          .append($('<li />', { 'class': 'menu-all-items group-filter' })
            .append($('<span />').text(_l("All items"))))
          .append($('<li />', { 'class': 'menu-new-items group-filter', 'data-value': 'unread' })
            .append($('<span />').text(_l("New items"))))
          .append($('<li />', { 'class': 'menu-starred-items group-filter', 'data-value': 'star' })
            .append($('<span />').text(_l("Starred")))))
        .append($('<ul />', { 'id': 'menu-settings', 'class': 'menu' })
          .append($('<li />', { 'class': 'menu-show-sidebar checkable' })
            .append($('<span />').text(_l("Show sidebar"))))
          .append($('<li />', { 'class': 'menu-show-all-subs checkable' })
            .append($('<span />').text(_l("Show all subscriptions")))))
        .append($('<ul />', { 'id': 'menu-folder', 'class': 'menu' })
          .append($('<li />', { 'class': 'menu-subscribe' }).text(_l("Subscribe…")))
          .append($('<li />', { 'class': 'menu-rename' }).text(_l("Rename…")))
          .append($('<li />', { 'class': 'menu-delete' }).text(_l("Delete…"))))
        .append($('<ul />', { 'id': 'menu-root', 'class': 'menu' })
          .append($('<li />', { 'class': 'menu-create-folder' }).text(_l("New folder…")))
          .append($('<li />', { 'class': 'menu-subscribe' }).text(_l("Subscribe…"))))
        .append($('<ul />', { 'id': 'menu-leaf', 'class': 'menu' })
          .append($('<li />', { 'class': 'menu-rename' }).text(_l("Rename…")))
          .append($('<li />', { 'class': 'menu-unsubscribe' }).text(_l("Unsubscribe…"))));
    },
    'initHelp': function() {
      var categories = [{
        'title': _l('Navigation'),
        'shortcuts': [
          { keys: _l('j/k'),       action: _l('open next/previous article') },
          { keys: _l('n/p'),       action: _l('scan next/previous article') },
          { keys: _l('Shift+n/p'), action: _l('scan next/previous subscription') },
          { keys: _l('Shift+o'),   action: _l('open subscription or folder') },
          { keys: _l('g, then a'), action: _l('open subscriptions') },
        ]}, {
        'title': _l('Application'),
        'shortcuts': [
          { keys: _l('r'), action: _l('refresh') },
          { keys: _l('u'), action: _l('toggle navigation mode') },
          { keys: _l('a'), action: _l('add subscription') },
          { keys: _l('?'), action: _l('help') },
        ]}, {
        'title': _l('Articles'),
        'shortcuts': [
          { keys: _l('m'),       action: _l('mark as read/unread') },
          { keys: _l('s'),       action: _l('star article') },
          { keys: _l('v'),       action: _l('open link') },
          { keys: _l('o'),       action: _l('open article') },
          // { keys: _l('t'),       action: _l('tag article') },
          // { keys: _l('l'),       action: _l('like article') },
          { keys: _l('Shift+a'), action: _l('mark all as read') },
        ]}];

      var maxColumns = 2; // Number of columns in the resulting table

      // Build the table
      var $table = $('<table/>');
      for (var i = 0, n = categories.length; i < n; i += maxColumns) {
        var keepGoing = true;
        for (var k = -1; keepGoing; k++) {
          var $row = $('<tr/>');
          $table.append($row);
          keepGoing = false;

          for (var j = 0; j < maxColumns && i + j < n; j++) {
            var category = categories[i + j];

            if (k < 0) { // Header
              $row.append($('<th/>', { 'colspan': 2 })
                .text(category.title));

              keepGoing = true;
            } else if (k < category.shortcuts.length) {
              $row.append($('<td/>', { 'class': 'sh-keys' })
                .text(category.shortcuts[k].keys + ':'))
              .append($('<td/>', { 'class': 'sh-action' })
                .text(category.shortcuts[k].action));

              keepGoing = true;
            } else { // Empty cell
              $row.append($('<td/>', { 'colspan': 2 }));
            }
          }
        }
      }

      $('body').append($('<div />', { 'class': 'shortcuts' }).append($table));
    },
    'initShortcuts': function() {
      $(document)
        .bind('keypress', '', function() {
          $('.shortcuts').hide();
          $('#floating-nav').hide();
          $$menu.hideAll();
        })
        .bind('keypress', 'n', function() {
          ui.selectArticle(1);
        })
        .bind('keypress', 'p', function() {
          ui.selectArticle(-1);
        })
        .bind('keypress', 'j', function() {
          ui.openArticle(1);
        })
        .bind('keypress', 'k', function() {
          ui.openArticle(-1);
        })
        .bind('keypress', 'o', function() {
          ui.openArticle(0);
        })
        .bind('keypress', 'r', function() {
          refresh();
        })
        .bind('keypress', 's', function() {
          if ($('.entry.selected').length)
            $('.entry.selected').data('entry').toggleStarred();
        })
        .bind('keypress', 'm', function() {
          if ($('.entry.selected').length)
            $('.entry.selected').data('entry').toggleUnread();
        })
        .bind('keypress', 'shift+n', function() {
          ui.highlightFeed(1);
        })
        .bind('keypress', 'shift+p', function() {
          ui.highlightFeed(-1);
        })
        .bind('keypress', 'shift+o', function() {
          if ($('.subscription.highlighted').length) {
            $('.subscription.highlighted')
              .removeClass('highlighted')
              .data('subscription').select();
          }
        })
        .bind('keypress', 'g', function() {
          lastGPressTime = new Date().getTime();
        })
        .bind('keypress', 'a', function() {
          if (ui.isGModifierActive())
            $('.subscription.root')
              .data('subscription').select();
          else
            ui.subscribe();
        })
        .bind('keypress', 'u', function() {
          ui.toggleSidebar();
        })
        .bind('keypress', 'v', function() {
          if ($('.entry.selected').length)
            $('.entry.selected').find('.entry-link')[0].click();
        })
        // .bind('keypress', 'l', function()
        // {
        //   if ($('.entry.selected').length)
        //     toggleLiked($('.entry.selected'));
        // })
        // .bind('keypress', 't', function()
        // {
        //   editTags($('.entry.selected'));
        // })
        .bind('keypress', 'shift+a', function() {
          ui.markAllAsRead();
        })
        .bind('keypress', 'shift+?', function() {
          $('.shortcuts').show();
        });
    },
    'initModals': function() {
      $('.modal-blocker').hide();
      $('.modal').hide();

      $.fn.showModal = function(show) {
        if (!$(this).hasClass('modal'))
          return;

        if (show) {
          $('.modal-blocker').show();
          $(this).show();
        } else {
          $('.modal-blocker').hide();
          $(this).hide();
        }
      };

      $('.modal-cancel').click(function() {
        $(this).closest('.modal').showModal(false);
        return false;
      });
    },
    'showImportSubscriptionsModal': function() {
      $('#import-subscriptions').find('form')[0].reset();
      $('#import-subscriptions').showModal(true);
    },
    'showAbout': function() {
      $('#about').showModal(true);
    },
    'isGModifierActive': function() {
      return new Date().getTime() - lastGPressTime < 1000;
    },
    'toggleSidebar': function(showSidebar) {
      if (typeof showSidebar === 'undefined')
        showSidebar = $('body').hasClass('floated-nav');

      $('body').toggleClass('floated-nav', !showSidebar);
      $('.menu-show-sidebar').setChecked(showSidebar);

      if ($('body').hasClass('floated-nav'))
        $('#floating-nav').append($('.feeds-container'));
      else
        $('#reader').prepend($('.feeds-container'));

      $.cookie('show-sidebar', showSidebar);
    },
    'toggleReadSubscriptions': function(showAllSubscriptions) {
      if (typeof showAllSubscriptions === 'undefined')
        showAllSubscriptions = $('body').hasClass('hide-read-subs');

      $('body').toggleClass('hide-read-subs', !showAllSubscriptions);
      $('.menu-show-all-subs').setChecked(showAllSubscriptions);

      $.cookie('show-all-subs', showAllSubscriptions);
    },
    'updateUnreadCount': function() {
      // Update the 'new items' caption in the dropdown to reflect
      // the unread count

      var selectedSubscription = getSelectedSubscription();
      var caption;

      if (!selectedSubscription || selectedSubscription.unread === null)
        caption = _l("New items");
      else if (selectedSubscription.unread == 0)
        caption = _l("No new items");
      else
        caption = _l("%1$s new item(s)", [selectedSubscription.unread]);

      $('.menu-new-items').setTitle(caption);

      // Update the title bar

      var root = subscriptionMap[""];
      var title = 'Gofr';

      if (root.unread > 0)
        title += ' (' + root.unread + ')';

      document.title = title;
    },
    'highlightFeed': function(which, scrollIntoView) {
      var $highlighted = $('.subscription.highlighted');
      if (!$highlighted.length)
        $highlighted = $('.subscription.selected');

      var $next = null;

      if (which < 0) {
        var $allFeeds = $('#subscriptions .subscription').not('.no-unread');
        var highlightedIndex = $allFeeds.index($highlighted);

        if (highlightedIndex - 1 >= 0)
          $next = $($allFeeds[highlightedIndex - 1]);
      } else if (which > 0) {
        if ($highlighted.length < 1)
          $next = $('#subscriptions .subscription:first').not('.no-unread');
        else {
          var $allFeeds = $('#subscriptions .subscription').not('.no-unread');
          var highlightedIndex = $allFeeds.index($highlighted);

          if (highlightedIndex + 1 < $allFeeds.length)
            $next = $($allFeeds[highlightedIndex + 1]);
        }
      }

      if ($next) {
        $('.subscription.highlighted').removeClass('highlighted');
        $next.addClass('highlighted');

        scrollIntoView = (typeof scrollIntoView !== 'undefined') ? scrollIntoView : true;
        if (scrollIntoView)
          $('.subscription.highlighted').scrollintoview({ duration: 0});
      }
    },
    'selectArticle': function(which, scrollIntoView) {
      if (which < 0) {
        if ($('.entry.selected').prev('.entry').length > 0)
          $('.entry.selected')
            .removeClass('selected')
            .prev('.entry')
            .addClass('selected');
      } else if (which > 0) {
        var $next = null;
        var $selected = $('.entry.selected');

        if ($selected.length < 1)
          $next = $('#entries .entry:first');
        else
          $next = $selected.next('.entry');

        $('.entry.selected').removeClass('selected');
        $next.addClass('selected');

        if ($next.next('.entry').length < 1)
          $('.next-page').click(); // Load another page - this is the last item
      }

      scrollIntoView = (typeof scrollIntoView !== 'undefined') ? scrollIntoView : true;
      if (scrollIntoView)
        $('.entry.selected').scrollintoview({ duration: 0});
    },
    'openArticle': function(which) {
      this.selectArticle(which, false);

      if (!$('.entry-content', $('.entry.selected')).length || which === 0)
        $('.entry.selected')
          .click()
          .scrollintoview();
    },
    'collapseAllEntries': function() {
      $('.entry.open').removeClass('open');
      $('.entry .entry-content').remove();
    },
    'showToast': function(message, isError) {
      if (message) {
        $('#toast span').text(message);
        $('#toast').attr('class', isError ? 'error' : 'info');

        if ($('#toast').is(':hidden')) {
          $('#toast')
            .fadeIn()
            .delay(8000)
            .fadeOut('slow'); 
        }
      }
    },
    'subscribe': function(parentFolder) {
      var url = prompt(_l("Site or feed URL:"));
      if (url) {
        if (url.indexOf('http://') != 0 && url.indexOf('https://') != 0)
          url = 'http://' + url;

        if (!parentFolder)
          parentFolder = getRootSubscription();

        parentFolder.subscribe(url);
      }
    },
    'rename': function(subscription) {
      var newName = prompt(_l("New name:"), subscription.title);
      if (newName && newName != subscription.title)
        subscription.rename(newName);
    },
    'createFolder': function() {
      var folderName = prompt(_l('Name of folder:'));
      if (folderName) {
        $.post('createFolder', {
          folderName : folderName,
        },
        function(response) {
          resetSubscriptionDom(response, false);
        }, 'json');
      }
    },
    'unsubscribe': function(subscription) {
      if (confirm(_l("Unsubscribe from %s?", [subscription.title])))
        subscription.unsubscribe();
    },
    'markAllAsRead': function() {
      var subscription = getSelectedSubscription();
      if (subscription == null || subscription.unread < 1)
        return;

      if (subscription.unread > 10 && !confirm(_l("Mark %s messages as read?", [subscription.unread])))
        return;

      var filter = $('.group-filter.selected-menu-item').data('value');
      subscription.markAllAsRead(filter);
    },
    'removeFolder': function(folder) {
      if (!confirm(_l("You will be unsubscribed from all subscriptions in this folder. Delete %s?", [folder.title])))
        return;

      folder.removeFolder();
    },
    'removeSubscriptionEntries': function(subscription) {
      $('#entries .entry').each(function() {
        var $entry = $(this);
        var entry = $entry.data('entry');

        if (entry.source == subscription.id)
          $entry.remove();
      });
    },
    'onEntryListUpdate': function() {
      var $centerMessage = $('.center-message');
      if ($('#entries .entry').length)
        $centerMessage.hide();
      else {
        // List of entries is empty
        $centerMessage.empty();
        $('.next-page').remove();

        if ($('.subscription').length <= 1) {
          // User has no subscriptions (root node doesn't count)
          $centerMessage
            .append($('<p />')
              .text(_l("You have not subscribed to any feeds.")))
            .append($('<p />')
              .append($('<a />', { 'href': '#' })
                .text(_l("Subscribe"))
                .click(function() {
                  ui.subscribe();
                  return false;
                }))
              .append($('<span />')
                .text(_l(" or ")))
              .append($('<a />', { 'href': '#' })
                .text(_l("Import subscriptions"))
                .click(function() {
                  ui.showImportSubscriptionsModal();
                  return false;
                })));
        } else {
          // User has at least one (non-root) subscription
          $centerMessage
            .append($('<p />')
              .text(_l("No items are available for the current view.")));

          if (!$('#menu-filter').isSelected('.menu-all-items')) {
            // Something other than 'All items' is selected
            // Show a toggle link
            $centerMessage
              .append($('<p />')
                .append($('<a />', { 'href' : '#' })
                  .text(_l("Show all items"))
                  .click(function() {
                    var selectedSubscription = getSelectedSubscription();
                    if (selectedSubscription != null) {
                      $('#menu-filter').selectItem('.menu-all-items');
                      selectedSubscription.refresh();
                    }

                    return false;
                  })));
          }
        }

        $centerMessage.show();
      }
    },
  };

  var getRootSubscription = function() {
    if ($('.subscription.root').length > 0)
      return $('.subscription.root').data('subscription');

    return null;
  };

  var getSelectedSubscription = function() {
    if ($('.subscription.selected').length > 0)
      return $('.subscription.selected').data('subscription');

    return null;
  };

  var generateSubscriptionMap = function(userSubs) {
    var map = { "": [] };
    var fmap = { };
    var idCounter = 0;

    // Create a combined list of folders & subscriptions
    $.each(userSubs.folders, function(index, folder) {
      folder.domId = 'sub-' + idCounter++;
      folder.link = null;
      folder.unread = 0;

      // Inject methods
      for (var name in folderMethods)
        folder[name] = folderMethods[name];

      if (folder.isRoot())
        folder.title = _l("All items");

      if (!map[folder.id])
        map[folder.id] = [];

      fmap[folder.id] = folder;
      map[""].push(folder);
    });

    var root = fmap[""];
    $.each(userSubs.subscriptions, function(index, subscription) {
      subscription.domId = 'sub-' + idCounter++;

      for (var name in subscriptionMethods)
        subscription[name] = subscriptionMethods[name];

      if (!subscription.parent)
        map[""].push(subscription);
      else {
        fmap[subscription.parent].unread += subscription.unread;
        map[subscription.parent].push(subscription);
      }

      root.unread += subscription.unread;
    });

    $.each(map, function(parentId, children) {
      // Sort the list of children by title
      children.sort(function(a, b) {
        var aTitle = a.title.toLowerCase();
        var bTitle = b.title.toLowerCase();

        if (a.isRoot())
          return -1;
        else if (b.isRoot())
          return 1;

        if (aTitle < bTitle)
          return -1;
        else if (aTitle > bTitle)
          return 1;

        return 0;
      });
    });

    delete fmap;
    
    return map;
  };

  var resetSubscriptionDom = function(userSubscriptions, reloadItems) {
    var selectedSubscription = getSelectedSubscription();
    var selectedSubscriptionId = null;

    if (selectedSubscription != null)
      selectedSubscriptionId = selectedSubscription.id;

    $('#subscriptions').empty();

    if (subscriptionMap != null)
      delete subscriptionMap;

    subscriptionMap = {};

    var subMap = generateSubscriptionMap(userSubscriptions);
    var createSubDom = function(subscription) {
      var $subscription = $('<li />', { 'class' : 'subscription ' + subscription.domId })
        .data('subscription', subscription)
        .append($('<div />', { 'class' : 'subscription-item' })
          .append($('<span />', { 'class' : 'chevron' })
            .click(function(e) {
              var $menu = $('#menu-' + subscription.getType());
              $menu.openMenu(e.pageX, e.pageY, subscription.id);
              e.stopPropagation();
            }))
          .append($('<div />', { 'class' : 'subscription-icon' }))
          .append($('<span />', { 'class' : 'subscription-title' })
            .text(subscription.title))
          .attr('title', subscription.title)
          .append($('<span />', { 'class' : 'subscription-unread-count' }))
          .click(function() {
            subscription.select();
          }))
        .bind('contextmenu', function(e) {
          var $menu = $('#menu-' + subscription.getType());
          $menu.openMenu(e.pageX, e.pageY, subscription.id);
          return false;
        });

      if (!subscription.isFolder()) {
        // Drag-and-drop code
        $subscription
          .mousedown(function(e) {
            var $elem = $(document.elementFromPoint(e.pageX, e.pageY)).closest('.subscription');
            if (!$elem.length)
              return;

            $('#subscriptions').mousemove(function(e) {
              if (!$dragSource) {
                $dragSource = $elem;
                $dragSource.addClass('dragged');
                dragDestination = null;

                return;
              }

              var dragSource = $dragSource.data('subscription');
              var $hoveredElement = $(document.elementFromPoint(e.pageX, e.pageY));

              if ($hoveredElement) {
                var $sub = $hoveredElement.closest('.subscription');
                var sub = $sub.data('subscription');

                var $newParent = null;
                var newParentId = null;

                if ($sub.is('li.folder')) {
                  $newParent = $sub.children('ul:first');
                  newParentId = sub.id;
                  dragDestination = sub;
                } else if (sub != null) {
                  $newParent = $sub.closest('ul');
                  newParentId = sub.parent;
                  dragDestination = subscriptionMap[sub.parent || ''];
                } else {
                  return false;
                }

                $('#subscriptions .dragging').remove();

                if (newParentId != dragSource.parent) {
                  var $followingElement = null;
                  var $clone = $dragSource.clone().removeClass('dragged').addClass('dragging');

                  $newParent.children('li').each(function() {
                    var $child = $(this);
                    var child = $child.data('subscription');

                    if (dragSource.title.toUpperCase() < child.title.toUpperCase()) {
                      $followingElement = $child;
                      return false;
                    }
                  });

                  if ($followingElement != null)
                    $clone.insertBefore($followingElement);
                  else
                    $clone.appendTo($newParent);
                }
              }
            });

            return false;
          });
      }

      return $subscription.addClass(subscription.getType());
    };

    var buildDom = function($parent, subscriptions) {
      $.each(subscriptions, function() {
        var subscription = this;
        var $subscription = createSubDom(subscription);

        $parent.append($subscription);
        if (subscription.id) {
          var children = subMap[subscription.id];
          if (children) {
            var $child = $('<ul />');
            $subscription.append($child);

            buildDom($child, children);
          }
        }

        subscriptionMap[subscription.id] = subscription;
        subscription.syncView();

        if (selectedSubscriptionId == subscription.id)
          selectedSubscription = subscription;
        else if (selectedSubscriptionId == null) {
          if (subscription.isFolder() && subscription.isRoot())
            selectedSubscription = subscription;
        }
      });
    };

    buildDom($('#subscriptions'), subMap[""]);

    selectedSubscription.select(reloadItems);
  };

  var loadSubscriptions = function() {
    $.getJSON('subscriptions', {
    })
    .success(function(response) {
      resetSubscriptionDom(response);
    });
  };

  var refresh = function() {
    loadSubscriptions();
  };

  var initChannels = function() {
    clientId = Math.random() + "";

    $.post('initChannel', {
      'client': clientId,
    },
    function(response) {
      channel = new goog.appengine.Channel(response.token);
      socket = channel.open();

      socket.onopen = function() {
        if (console && console.debug)
          console.debug("Channel open");
      };
      socket.onclose = function() {
        // Reconnect
        if (console && console.debug) {
          console.debug("Channel closed");
          initChannels();
        }
      };
      socket.onmessage = function(m) {
        var obj = $.parseJSON(m.data);
        if (obj.error)
          ui.showToast(obj.error, true);
        else {
          if (obj.message)
            ui.showToast(obj.message, false);
          if (obj.refresh)
            refresh();
        }
      };
      socket.onerror = function(error) {
        if (console && console.debug)
          console.debug("Received an error: " + error);
      };
    }, 'json');
  };

  ui.init();
  initChannels();

  loadSubscriptions();
});
