# goStaticEnv

This project is a fork of [mmguero-dev/goStatic](https://github.com/mmguero-dev/goStatic), with additional features and improvements. The name has been updated to **goStaticEnv** to reflect its enhanced capabilities.

[![Docker Image Version (latest by date)](https://img.shields.io/docker/v/mallox/go-static-env?sort=date)](https://hub.docker.com/r/mallox/go-static-env/tags)
[![Tests](https://github.com/mallocator/goStaticEnv/actions/workflows/test.yml/badge.svg)](https://github.com/mallocator/goStaticEnv/actions/workflows/test.yml)

A really small, multi-arch, static web server for Docker

## The goal

My goal is to create to smallest docker container for my web static files. The advantage of Go is that you can generate a fully static binary, so that you don't need anything else.

### Wait, I've been using old versions of GoStatic and things have changed!

Yeah, decided to drop support of unsecured HTTPS. Two-years ago, when I started GoStatic, there was no automatic HTTPS available. Nowadays, thanks to Let's Encrypt, it's really easy to do so. If you need HTTPS, I recommend [caddy](https://caddyserver.com).

## About this fork

goStaticEnv extends the original goStatic with support for environment variable substitution in served files. This allows you to inject environment-specific values directly into your static content at runtime, making it ideal for containerized and cloud-native deployments.

## Environment Variable Substitution

**Feature:**

You can use environment variables in your static files using the syntax `${VARNAME}` or `${VARNAME:=default}`. When a file is served, goStaticEnv will replace these placeholders with the value of the corresponding environment variable, or with the provided default if the variable is not set.

**Usage Example:**

Suppose your HTML file contains:

```html
<script>
  window.API_URL = "${API_URL:=https://api.example.com}";
</script>
```

If you run the server with `API_URL` set in the environment, it will be replaced. If not, the default will be used.

**How it works:**
- `${VARNAME}`: Replaced with the value of `VARNAME` if set, otherwise left as-is.
- `${VARNAME:=default}`: Replaced with the value of `VARNAME` if set, otherwise replaced with `default`.

**Validation:**
At startup, goStaticEnv will scan your static files and report an error if any required environment variables (without a default) are missing. You can use the `--allow-missing-env` flag to start the server with warnings instead of exiting when environment variables are missing.

**Directory Filtering:**
You can control which directories are scanned for environment variables using the include/exclude flags:
- `--env-include-dirs`: Only scan the specified directories (comma-separated, relative to base path)
- `--env-exclude-dirs`: Skip the specified directories when scanning (comma-separated, relative to base path)

**Pattern Matching:**
Directory patterns support both simple names and glob-style wildcards:
- **Simple patterns**: `docs` matches `docs`, `docs/api`, `src/docs`, etc. (intuitive prefix matching)
- **Glob patterns**: Use `*`, `?`, and `[]` for flexible matching (powered by Go's `filepath.Match`)
  - `*` matches any sequence of characters (except `/`)
  - `?` matches any single character
  - `[abc]` matches any character in the set

**Verified Examples:**
```bash
# Simple patterns (verified working)
./goStaticEnv --env-include-dirs "src,public"           # Include only src/ and public/ directories
./goStaticEnv --env-exclude-dirs "node_modules,vendor"  # Exclude node_modules and vendor anywhere

# Glob patterns with wildcards (all verified working)
./goStaticEnv --env-exclude-dirs "test-*"               # Exclude test-unit, test-integration, etc.
./goStaticEnv --env-exclude-dirs "*-backup"             # Exclude old-backup, data-backup, etc.
./goStaticEnv --env-include-dirs "*.tmp"                # Include config.tmp, cache.tmp, etc.
./goStaticEnv --env-include-dirs "cache-?"              # Include cache-1, cache-2, etc.

# Complex path patterns (verified working)
./goStaticEnv --env-include-dirs "docs/v*"              # Include docs/v1, docs/v2, docs/v1-draft, etc.

# Mixed include/exclude patterns (verified working)
./goStaticEnv --env-include-dirs "src,docs/v*" --env-exclude-dirs "docs/v1-*"  # Include src and docs/v* but exclude docs/v1-*

# Common use cases (all verified working)
./goStaticEnv --env-exclude-dirs "node_modules,vendor,.git,test-*,*-backup"    # Exclude common build/temp directories
./goStaticEnv --env-include-dirs "src,public,docs"                            # Only scan specific source directories
```

## Features

* A fully static web server embedded in a `SCRATCH` image
* No framework
* Web server built for Docker
* Light container
* More secure than official images (see below)
* Log enabled
* Specify custom response headers per path and filetype [(info)](./docs/header-config.md)
* **NEW:** Environment variable substitution in static files

## Why?

Because the official Golang image is wayyyy too big (around 1/2Gb as you can see below) and could be insecure.

For me, the whole point of containers is to have a light container...
Many links should provide you with additional info to see my point of view:

* [Over 30% of Official Images in Docker Hub Contain High Priority Security Vulnerabilities](http://www.banyanops.com/blog/analyzing-docker-hub/)
* [Create The Smallest Possible Docker Container](http://blog.xebia.com/2014/07/04/create-the-smallest-possible-docker-container/)
* [Building Docker Images for Static Go Binaries](https://medium.com/@kelseyhightower/optimizing-docker-images-for-static-binaries-b5696e26eb07)
* [Small Docker Images For Go Apps](https://www.ctl.io/developers/blog/post/small-docker-images-for-go-apps)

## How to use

```bash
docker run -d -p 80:8043 -v path/to/website:/srv/http -e API_URL=https://api.example.com --name goStatic ghcr.io/mmguero-dev/gostatic
```

## Usage

```bash
./goStatic --help
Usage of ./goStatic:
  -allow-missing-env
        Allow server to start with warnings when environment variables are missing, instead of exiting with fatal error
  -append-header HeaderName:Value
        HTTP response header, specified as HeaderName:Value that should be added to all responses.
  -context string
        The 'context' path on which files are served, e.g. 'doc' will serve the files at 'http://localhost:<port>/doc/'
  -default-user-basic-auth string
        Define the user (default "gopher")
  -enable-basic-auth
        Enable basic auth. By default, password are randomly generated. Use --set-basic-auth to set it.
  -enable-health
        Enable health check endpoint. You can call /health to get a 200 response. Useful for Kubernetes, OpenFaas, etc.
  -enable-logging
        Enable log request
  -env-exclude-dirs string
        Comma-separated list of directories to exclude when scanning for environment variables (relative to base path)
  -env-include-dirs string
        Comma-separated list of directories to include when scanning for environment variables (relative to base path)
  -fallback string
        Default fallback file. Either absolute for a specific asset (/index.html), or relative to recursively resolve (index.html)
  -header-config-path string
        Path to the config file for custom response headers (default "/config/headerConfig.json")
  -https-promote
        All HTTP requests should be redirected to HTTPS
  -password-length int
        Size of the randomized password (default 16)
  -path string
        The path for the static files (default "/srv/http")
  -port int
        The listening port (default 8043)
  -set-basic-auth string
        Define the basic auth user string. Form must be user:password
  -basic-auth-user
        Define the basic auth username
  -basic-auth-pass
        Define the basic auth password
```

### Fallback

The fallback option is principally useful for single-page applications (SPAs) where the browser may request a file, but where part of the path is in fact an internal route in the application, not a file on disk. goStatic supports two possible usages of this option:

1. Using an absolute path so that all not found requests resolve to the same file
2. Using a relative file, which searches up the tree for the specified file

The second case is useful if you have multiple SPAs within the one filesystem. e.g., */* and */admin*.

## Build

### Docker images

```bash
docker buildx create --use --name=cross
docker buildx build --platform=linux/amd64,linux/arm64,linux/arm/v5,linux/arm/v6,linux/arm/v7,darwin/amd64,darwin/arm64,windows/amd64 .
```
