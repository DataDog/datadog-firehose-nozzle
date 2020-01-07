# Development

## Setup

Dependencies are managed using Glide. 
In order to build, install or run tests, first [install glide](https://github.com/Masterminds/glide).
Then, pull dependencies by calling:

```
glide install
```

## Testing

You need [ginkgo](http://onsi.github.io/ginkgo/) to run the tests. 
Don't forget to setup your `GOPATH` and `PATH` variables:

```
export PATH=$PATH:$GOPATH/bin
```

Tests can be executed with:

```
ginkgo -r
```
