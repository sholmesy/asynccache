Asyncronous Cache Refreshing in Go
==================================

HTTP Middleware to manage cached content, partly inspired by [Django-Cacheback](https://github.com/codeinthehole/django-cacheback)

Provides similar functionality to [proxy_cache_background_update](http://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_cache_background_update) from NGINX.
Useful if you're running a single go binary without sitting behind NGINX.

Serves cached data up until the stale threshold is reached, at which point it will trigger an asynchronous refresh.

Usage
=====
- Run a redis instance `redis-server`.
- Create a basic go http server:

```
cache := middleware.NewAsyncCache()
http.Handle("/", middleware.AsyncCacheMiddleware(http.HandlerFunc(handler), cache))
http.ListenAndServe(":8080", nil)
```
