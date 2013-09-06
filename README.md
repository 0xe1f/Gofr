Gofr
==========

*Gofr* (pronounced "gopher") is a Feed Reader (Google Reader clone) for [Google App Engine] [1]. It grew out of my frustration with [grr's] [2] relational database backend, and my inability to optimize it beyond (still) unsatisfactory results. 

_Gofr_ is written in [Go] [3], and uses the [Google Cloud Datastore] [4]. Like _grr_, it relies heavily on [JavaScript] [5]. However, unlike _grr_, because of the features provided by the App Engine, _Gofr_ will likely not require a dedicated scheduled task - instead, it will update feeds on-demand.

This application is currently in pre-alpha state, and not completely usable.

  [1]: https://developers.google.com/appengine/
  [2]: https://github.com/melllvar/grr/
  [3]: http://golang.org/
  [4]: https://developers.google.com/datastore/
  [5]: http://en.wikipedia.org/wiki/JavaScript