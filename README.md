# crsym: The Chrome Symbolizer

crsym is a tool that can parse crash report data and symbolize it using [Google Breakpad](https://code.google.com/p/google-breakpad/) symbol files (produced by the dump_syms program). While most crash reports should be caught by your product's automated crash reporting system (e.g. Breakpad), sometimes crash data comes from other sources. It is crash reports from these other sources that crsym specializes in symbolizing.

## Types of Crash Reports

The crsym tool has parsers for the following kinds of crash reports:

* Apple crash and hang reports (typically found in ~/Library/Logs/DiagnosticReports).
* Breakpad minidumps formatted using mimidump_stackwalk.
* Android crash reports written to logcat.
* Arbitrary addresses, where the module load address is specified by the user.

## Code Organization

In the initial open source release, only three libraries are provided and not a buildable server. The server component used internally by Google relies on non-public infrastructure and thus cannot be open sourced, but it is a goal of the project to reuse the libraries to create an open-source version of the server.

The first library is the `breakpad` library, and it provides a parser for Breakpad symbol files produced by `dump_syms`. It also defines interfaces for "backends" which can vend these symbol files, from e.g. an RPC service or the file system. Currently no implementation of these interfaces exist in the open-source project.

The second library is `parser`, which defines an interface `parser.Parser`. It contains a collection of Parsers, one for each type listed above, along with a battery of test data.

The third library is the `frontend` library, which contains handlers for an (yet unwritten) HTTP server, and the actual web interface.

See the TODO file for the active tasks for the open source project.
