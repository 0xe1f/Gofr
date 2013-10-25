Gofr
==========

**Gofr** is a Feed Reader (Google Reader clone) for [Google App Engine] [1]. It grew out of my frustration with the relational backend of [grr] [2] and my inability to optimize it beyond unsatisfactory results. 

Gofr is written in [Go] [3], and uses the [Google Cloud Datastore] [4].

![Screenshot](http://i.imgur.com/FbVKfsJ.png "Screenshot")

Installation
------------

To run locally on development server:

1. Clone the repository: `git clone https://github.com/melllvar/Gofr.git`
2. Install the [go-charset](https://code.google.com/p/go-charset/) library: `go get code.google.com/p/go-charset/charset`
3. Run the development server: `dev_appserver.py Gofr/`

To deploy:

1. Clone the repository: `git clone https://github.com/melllvar/Gofr.git`
2. Change into the new directory: `cd Gofr`
3. Edit [app.yaml](app.yaml) and change the name of the application (initially "gofr-io") to one of your choosing
4. Deploy to production: `appcfg.py update .`

Dev Server Notes
----------------

When running in production, Gofr routinely (every 10 minutes, configurable in [cron.yaml](cron.yaml)) runs a cron job to update feeds. Since the development server does not support cron jobs, the feeds will never update by themselves. To update the feeds, log in to the application as an Administrator, and open the cron job URL in a web browser: `http://localhost:8080/cron/updateFeeds`.

  [1]: https://developers.google.com/appengine/
  [2]: https://github.com/melllvar/grr/
  [3]: http://golang.org/
  [4]: https://developers.google.com/datastore/
  [5]: http://en.wikipedia.org/wiki/JavaScript
