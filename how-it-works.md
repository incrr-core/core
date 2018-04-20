# Incrr: How it Works

There are three main parts to the system:

- Groupcache: the atomic engine of the system and provides the consistent hashing for HA
- Local datastore: to keep an inventory of what keys have been seen by the server, and what it thinks the current number the atomic increment for that key is at.
- Remote datastore: to keep every transaction for data backup and restore, and recovery of evicted keys

## Using Groupcache to NOT serve huge files

Groupcache is a distributed Go cache library, the first commit for Groupcache was 5 years ago. In July of 2013, just after go1.1 was released. Groupcache is a memcache-like replacement that does automatic key eviction, doesn't support versioned keys, automatically replicates hot items and can be used as a library instead of a stand-alone server.

The original use case for Groupcache was to help serve large download files from within the Google infrastructure (think dl.google.com). This use case appears to still be the most prevalent use, according to the few blog posts or slides that can be found on using Groupcache. Most information tends to focus on the ability to use a cache to deliver large files efficiently.

However, Incrr is using Groupcache to serve a single number 8-bytes big. This is certainly much less than a typical Chrome executable or video file download size So why does Incrr use Groupcache? We are specifically using Groupcache for the "doesn't support versioned keys" property, which is another way of saying that things don't change. Which is also an important tenet of atomicity, the fact that things won't change under your feet without you knowing.

Googling around for how to implement atomic operations, generally yields the Compare-and-Swap (CAS) algorithm, which is also sometimes referred to Compare-and-Set. The basic way that CAS works is that a program that would like to change a function first must first read the value, then when it wants to change it, check to make sure that it's the same value and if so, make the change. The next program that would like to make the change needs to do the same thing. But if the value has changed before the time the program wants to make the change, then it needs to start over again. There are more detailed explanations on how CAS works on Wikipedia.

We're going to do something similar with Groupcache, it's more of a Set-and-Compare (SAC) operation. The way this works is; a program will have its own incrementing counter and it will attempt to set a key with a unique value, then compare the response with its unique value. If the key has not been previously set then when the values will be the same. If they are not the same then another program has claimed that value so increment your counter and repeat to see if you can claim the next value.

The main caveat is that the program should start incrementing at a number pretty close to the value that it wants to get back. So there is the practicality of knowing this, we'll use this later with the local datastore to keep that number. 

Back to Groupcache, so we know that once a key is set then it can't be changed and we can use our SAC scheme to check. But how do we get the unique value to Groupcache? One thing that Groupcache is really good at is serving files, files that a usually sitting in a storage blob, files that are not really changing much. Files that certainly are not changing on each call to Groupcache. Which is why the "one value per key" thing works really well with Groupcache. Well, the secret is in the "context".

Groupcache seems to have had a way to pass in context from the beginning (based on my light research of the source code). Becuase context is an interface we can simply pass in data that we want to get back through this channel and grab it during the function that Groupcache uses to serve the original value. Easy we can pass in unique values to check against, and see if they match then we can claim the number and respond, if not we can try the next incrementing number with different unique values and try again.

The Groupcache context is not the context.Context that was added to the standard library for the release of Go 1.7 just about 3 years later. I haven't seen any blog post or write-ups on how context is used for any current Groupcache products, but I would suspect it's mainly for lifecycle events. Lifecycle events are what context.Context handles a lot of for the current HTTP standard library. This is timeouts and cancellation events. 

```
 // Context optionally specifies a context for the server to use when it
 // receives a request.
 // If nil, the server uses a nil Context.
```

It's not a context for the Group function. But we can remedy that.

Ignoring the following warning in the HTTP standard library for RoundTripper

```
// RoundTrip should not modify the request, except for
// consuming and closing the Request's Body. RoundTrip may
// read fields of the request in a separate goroutine. Callers
// should not mutate the request until the Response's Body has
// been closed.
```

We can change add a context header to the request and ship the request that the server can use.

```
type myRoundTripper string
func (rt *myRoundTripper) RoundTrip(r *http.Request) (w *http.Response, err error) {
    r.Header.Add("x-context-header", string(rt)) // yeah, not supposed to do this...
    return http.DefaultTransport.RoundTrip(r)
}
...
cache.HTTPPool.Transport = func(ctx groupcache.Context) http.RoundTripper {
    var rt myRoundTripper = convertToString(ctx)
    return &rt
}
cache.HTTPPool.Context = func(r *http.Request) groupcache.Context {
    return groupcache.Context(r.Header.Get("x-context-header"))
}
```

The above is a little example of how we can ship a string based context to the other servers. So now we can ship our unique values to remote servers to see if we can get them back.

Now that we can use context to provide uniqe values what is an example:

```
{"id":"abcd", "ts":"123456789", "#":"%s"}
```

I first sent a JSON string that Group cache could fill in, which is the ID of the server sending this message, the timestamp of the value and the number that should be filled in by Groupcache. It requires that both pieces of information be present, the Key and the Value to make a valid response. Also, it allows the code to take the expensive operation of generating the JSON string once for any operation.

## The Local Datastore

For Incrr we use a memory mapped datastore to keep track of what keys are being used locally.

## The Remote Datastore

The remote store can gather unique keys and keep a transaction of available keys.