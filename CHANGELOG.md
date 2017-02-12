# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/) 
and this project adheres to [Semantic Versioning](http://semver.org/).

## [v0.2.0] - 2017-02-12
### Added

- Calculated Cache-Control for the resulting JSON response
- ETag
- Conditional transport to support conditional GET

### Changed

- Stop eviction when the given max memory size equals 0
- Mark cached responses outdated after a destructive request like POST, instead of clearing them
- Error JSON is now RFC7807 Problem Details aware

### Fix

- Fix cache index so that it contains HTTP method

## [v0.1.0] - 2017-01-01
### Added

- Embed transport to combine multiple JSON resources
- Concurrent JSON fetches
- Error JSON for failed fetches
- Cache transport for performance
- LRU cache eviction
- Support for non-JSON resources
