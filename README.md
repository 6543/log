# Observability API

I propose to create an industry standard on observability API in Go. Let outline major problems, first...

## Problem #1: Logger

The most popular loggers for Go are:

* Standard log (https://pkg.go.dev/log).
* logrus (https://github.com/Sirupsen/logrus)
* zap (https://github.com/uber-go/zap)

But there are also a lot of other loggers. All of them have different advantages and disadvantages. And basically there is no industry standard even on the interface of a logger.

## Problem #2: Injecting additional observability tooling

When an application evolves it often meets new challenges:

* The application become to complex and consists of multiple parts, and it becomes difficult to diagnose latencies (do not confuse with CPU profiling, latencies may be caused my network issues for example).
* In some places logging is not applicable by multiple reasons. For example it may generate too many logs (like millions entries per second).
* It becomes important to automatically look for regressions.

Thus it is required requires:

* To allow enabling of metrics (https://github.com/VictoriaMetrics/VictoriaMetrics/wiki/MetricsQL) (without rewriting the application).
* To allow enabling of tracing tools (https://opentracing.io/specification/) (without rewriting the application).
* Automatically monitor for new errors (https://sentry.io/welcome/).

And all these tooling (logger, metrics, tracing and error monitoring) has the same pattern: they have structured fields.

# Requirements

1. In contrast to current practices for observability in Go, it is required to apply “dependency injection” and abstract the observability tooling using interfaces (to be able to inject whatever implementation is required).
2. It should be possible define structured values gradually. Thus a context with fields should allow derivatives. Let’s call such approach “contextual” (see "Contextuality" below).
3. Usability in a system with multiple services processing a single request.

## Contextuality

There are different implementations of structured logging for Go, but not successful in practice (like zap) allows to define structured fields gradually. For example:

```go
func myFunc0(groupID int, zapLogger *zap.Logger) {
	zapLogger = zapLogger.With(zap.Int("group_id", groupID))
	...
	for _, userID := range group.UserIDs() {
		myFunc1(userID, zapLogger)
	}
	...
}
func myFunc1(userID int, zapLogger *zap.Logger) {
	zapLogger = zapLogger.With(zap.Int("user_id", userID))
	...
	zapLogger.Debug("user password is expired")
	...
}
```

This will create a log entry with at least two structured values: “`group_id`” and “`user_id`”. Or in logrus it will be:

```go
func myFunc0(groupID int, logrusLogger *logrus.Entry) {
	logrusLogger = logrusLogger.WithField("group_id", groupID)
	...
	for _, userID := range group.UserIDs() {
		myFunc1(userID, logrusLogger)
	}
	...
}
func myFunc1(userID int, logrusLogger *logrus.Entry) {
	logrusLogger = logrusLogger.WithField("user_id", userID)
	...
	logrusLogger.Debugf("user password is expired")
	...
}
```

Thus myFunc1 itself has no responsibility to know about group_id to log it.

# Candidates

We are known two candidates to solve the problems:

* [OpenTelemetry](https://opentelemetry.io/).
* [eXtended context "`xcontext`"](https://github.com/facebookincubator/contest/blob/784d571/pkg/xcontext/context.go#L67-L159).

## OpenTelemetry

The problems are:

* They currently support only metrics and tracing (https://github.com/open-telemetry/opentelemetry-go/blob/257ef7fc150fb5b37fa1b42c83134dd52685861e/README.md#export). And it is unclear how logger API (which is the most important one) will look like. Here is the discussion on this subject: https://github.com/open-telemetry/opentelemetry-go/pull/2010
* They have too complicated API (difficult to swallow right away by a gopher-newbie). This seems to be caused by attempt to provide be a completely SOLID-compliant solution, which is theoretically good, but in practice may frighten away users from using it. Good enough is better than perfect, and it seems better to sacrifice non-important design features to improve user experience.
* It looks like OpenTelemetry does not support “contextuality”.
* OpenTelemetry defines observability tooling as separate tools, instead of unifying them into joint entity “Observability” which:
    * Generalizes the “contextuality” property over all tools.
    * Provides placeholders to inject observability tools (like metrics) when it will be required.

## `xcontext`

The problem is that SOLID is not followed. The extended context is a light version of a [God Object](https://en.wikipedia.org/wiki/God_object).

# Solution

## Part 1: Logger interface

The properties of the logger:

* Dependency injection. The logger should be injectable (instead of a global instance) and abstract.
* It should be structured and contextual.
* It should support logging levels, and they also should be contextual.
* It should be extendable. It should be possible to attach “hooks“ to modify log entries, or to actually log them (to stdout, file, logstash, whatever).

### Part 1.1: Logger interface: structured contextual logging

It should be possible to gradually extend the context by creating derivatives (without modification the original context). There proposed two major approaches:
* Either the context is a part of the logger itself.
* Or logger is responsible for logging only, and logging context is a separate entity.

It would be somewhat more SOLID to use the second approach (see "single-responsibility principle"). But in practice it does not change anything except that an user will need to passthrough two entities through the whole call stack instead of one. Thus we propose to use the first approach:

```go
type Fields map[string]interface{}

type Logger interface {
	...
	WithField(fieldName string, fieldValue interface{})
	WithFields(fields Fields)
	WithStruct(interface{})
	...
}
```

The main downside here is that `map[string]interface{}` is too expensive in terms of CPU consumption. For example non-sugared zap has a [`Field` structure](https://github.com/uber-go/zap/blob/c23abee72d197be00f17816e336aca5c72c6f26a/zapcore/field.go#L103-L109) to avoid interface-boxing and allocing maps. And we can re-use the same approach:
```go
type Field struct {
	Key       string
	Integer   int64
	String    string
	Bytes     []byte
	Interface interface{}
}

type Fields []Field

type Logger interface {
	...
	WithValue(key string, value interface{})
	WithFields(fields Fields)
	WithMap(map[string]interface{})
	WithStruct(interface{})
	...
}
```

*[BTW, I had to use the same approach, when I [experimented](https://github.com/xaionaro-go/atomicmap/blob/96a7f1f95a7093253c3ca2a9dad34e56b63ea6a2/storage.go#L42-L43) with high-performance `map`-s in Go]*

An usage example:
```go
func myFunc(myLogger Logger) {
	...

	myLogger = myLogger.WithFields(logger.Fields{
		{
			Key: "user_id",
			Integer: userID,
		},
		{
			Key: "user_name",
			String: userName,
		}
	})

	...

	myLogger.WithField("error_msg", err.Error()).Debug("yay!")

	...

	anotherFunc(myLogger.WithStruct(rsaPubKey))
}
```

### Part 1.2: Logger interface: logging levels

There are two main approaches here: to define a method for each logging level or to generalize everything into single method:
```go
type Logger interface {
	...
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	...
}
```
vs
```go
type Logger interface {
	...
	Log(level Level, format string, args ...interface{})
	...
}
```

Despite the fact the generalized approach (with `Log`) provides much less code duplication, it provide worse user experience:
```go
myLogger.Log(logger.LevelDebug, "yay!")
```
is less readable than:
```go
myLogger.Debug("yay!")
```

And the logger is a thing which is very rarely modified, but used a lot. Thus it is proposed to define separate methods for each logging level.

And to make logging level contextual it is required to have setter and getter methods:
```go
WithLevel(level Level)
Level() Level
```

### Part 1.3: Logger interface: logging methods

Historically (since "C") a lot of programmers used to format+arguments API, like:
```
Printf(format string, args ...interface{})
```

The problem of this API is that it is oriented for non-structural output, while it is required to encourage users to log data in a structured way. But to allow smooth transition we still need to permit the API people used, to. Thus it is proposed to have two formats:
```go
Debugf(format string, args ...interface{})
```
and:
```go
Debug(values ...interface{})
```
If method `Debugf` is used then basically `fmt.Sprintf` is used to format the string and it is logged as is with key `message`.

If method `Debug` is used then depending on types of input values they are interpreted in a different way:
* If `Field` is provided then it is interpreted as `Field`.
* If a structure (or a pointer to a structure) is provided, then each struct field is interpreted as a separate value. The key of the value is determined from tag `log` or from struct field name (if the tag is not set).
* Everything rest is just converted to string and joined together into `message`.

If values is empty then log only those fields which were set in this context (without adding anything). Thus these variants are equivalent:
```go
myLogger.Debug("yay!")
```
```go
myLogger.WithValue("message", "yay!").Debug()
```

### Part 1.4: Logger interface: Flush()

Since logging is highly connected with IO and specifically "output" it is required to provide method "Flush()" to support buffered output (which is usually critical in high performance applications):
```go
type Logger interface {
	...
	Flush()
	...
}
```

### Part 1.5: Logger interface: extendable

It should be possible to add hooks to a `Logger` which are responsible for modifying/enriching the logged data and performing the actual logging. Therefore it is required to define how a logging entry looks like:
```go
type Entry interface {
	Timestamp time.Time
	Level Level
	Fields Fields
	Caller Caller
}
```
(see also semantically the same entity in [logrus](https://github.com/sirupsen/logrus/blob/v1.8.1/entry.go#L44-L70) and [zap](https://github.com/uber-go/zap/blob/v1.19.0/zapcore/entry.go#L146-L153))

Type `Caller` basically could be completely re-used from `zap` (it is already optimal):
```go
type Caller struct {
	Defined bool
	PC      uintptr
	File    string
	Line    int
}
```

In total a hook is:
```go
type Hook interface {
	Process(*Entry)
}
```

And in turn a `Logger` is extendable by these hooks:
```go
type Logger interface {
	...
	WithHooks(...Hook)
	Hooks() []Hook
	...
}
```
To remove all hooks: `myLogger = myLogger.WithHooks(nil)`.

### Part 1.6: Logger interface: in total

```go
type Field struct {
	Key       string
	Integer   int64
	String    string
	Bytes     []byte
	Interface interface{}
}

type Fields []Field

type Caller struct {
	Defined bool
	PC      uintptr
	File    string
	Line    int
}

type Entry interface {
	Timestamp time.Time
	Level Level
	Fields Fields
	Caller Caller
}

type Hook interface {
	Process(*Entry)
	Flush()
}

type Logger interface {
	Tracef(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	Trace(values ...interface{})
	Debug(values ...interface{})
	Info(values ...interface{})
	Warn(values ...interface{})
	Error(values ...interface{})
	Panic(values ...interface{})

	WithValue(key string, value interface{})
	WithFields(fields Fields)
	WithMap(map[string]interface{})
	WithStruct(interface{})
	Fields() Fields

	WithHooks(...Hook)
	Hooks() []Hook

	Flush()
}
```

## Part 2: Observability API

### Part 2.1: Observability API: basics

Currently there are known two approaches:
* [OpenTelemetry](https://opentelemetry.io/).
* [eXtended context "xcontext"](https://github.com/facebookincubator/contest/blob/784d571/pkg/xcontext/context.go#L67-L159).

OpenTelemetry does not provide required properties, while `xcontext` is designed in as somekind of a "God Object". To combine the best parts of the both approaches it is proposed to create a modular observability object, which does not know anything about specific observability tool, but unifies them together with support of contextual structured data.

```go
type Tool interface {
	WithValue(key string, value interface{})
	WithFields(fields Fields)
	WithMap(map[string]interface{})
	WithStruct(interface{})
	Fields() Fields
}

type ToolEntry struct {
	Key ToolKey
	Tool Tool
}

type Tools []ToolEntry

func (tools Tools) Get(key ToolKey) Tool {
	// linear search is faster than map if there are only few entries
	for _, toolEntry := range tools {
		if toolEntry.Key == key {
			return toolEntry.Tool
		}
	}
	return false
}

type Observer struct { ... }

func (obs *Observer) WithValue(key string, value interface{}) *Observer { ... }
func (obs *Observer) WithFields(fields Fields) *Observer { ... }
func (obs *Observer) WithMap(map[string]interface{}) *Observer { ... }
func (obs *Observer) WithStruct(interface{}) *Observer { ... }
func (obs *Observer) Fields() Fields { ... }
func (obs *Observer) WithTools(tools ...Tool) { ... }
func (obs *Observer) Tools() Tools
```

An example of usage:
```go
func main() {
	...
	obs := obs.NewObserver().WithTools(metrics.New(), logger.New()) // metrics are first, because they should be faster
	...
	obs = obs.WithValue("pid", os.Getpid())
	...
	myFunc(obs)
	...
}

func myFunc(obs *Observer) {
	defer tracer.FromObs(obs).Start("doingSomeStuff").Finish()
	...
	logger.FromObs(obs).Debug("yay!")
	...
}
```
This will resuls into a log entry with fields `{"pid":1234, "message":"yay!"}`. And it won't panic on `tracer` despite the fact it was not initialized, because `FromObs` is obligated to return a dummy implementation if no `tracer` is defined in the `Observer`.

### Part 2.2: Observability API: integrating with `context`

It is possible (but not necessary) to inject `Observer` into a standard context:
```go
package obscontext

import "context"

type ctxKey string
const ctxKeyObserver = ctxKey("observer")

func WithObs(ctx context.Context, obs *Observer) context.Context {
	return context.WithValue(ctx, ctxKeyObserver, obs)
}
func Obs(ctx context.Context) *Observer {
	observer := ctx.Value(ctxKeyObserver)
	if observer == nil {
		return obs.Default()
	}
	return observer.(*Observer)
}

func WithValue(ctx context.Context, key string, value interface{}) context.Context {
	observer := ctx.Value(ctxKeyObserver)
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, obs.WithValue(key, value))
}
func WithFields(ctx context.Context, fields Fields) context.Context { ... }
func WithMap(ctx context.Context, map[string]interface{}) context.Context { ... }
func WithStruct(ctx context.Context, interface{}) context.Context { ... }
```

It is proposed to implement support of contexts are a separate package, because there is no consensus on is this a right approach or not. It is a well-recognized pattern, but the arguments againts storing observer (usually specifically "Logger") in a [context](https://pkg.go.dev/context):
* A context should not contain "behavior".
* A context abstracts values to `interface{}` which effectively disables some runtime guarantees. And even if you will somehow avoid panics you still cannot validate at compile time that `Observer` will be reachable in a specific piece of code.

Counter-arguments:
* How many is stored in context, and how many is stored in a global singleton is decision for a specific Logger implementation. This approach does not enforce putting "behavior" to context.
* In this approach we require getters to return the default instance if the context has no `Observer`. This way it cannot cause runtime errors and guarantee a fallback mechanism to still do logs.

Thus to provide an user a choice use it or not to use we put this code into a separate package.

### Part 2.3: Observability API: TraceID

A distributed system requires to diagnose cross-service issues.

The most common way to solve this problem is to have a `TraceID` which is persistent for an end-user request through all logs from all services.

Thus `Observer` may also implement:
```go
func (obs *Observer) TraceID() string
func (obs *Observer) WithTraceID(traceID string) *Observer
```

Another challenges are:
* A request to a frontend service may leads to multiple requests to other services, which in turn may also multiply amount of requests. Thus a request processing is actually a tree-like process. And sometimes it is required diagnose a specific branch instead of the whole tree.
* A log entry may be related to multiple `TraceID`-s. For example, an user may send to requests to a job management service: one to create a job and another to cancel it. There could be a log entry which includes both `TraceID`-s.

Therefore it is proposed instead of the methods above to enforce methods:
```go
func (obs *Observer) TraceIDs() []string
func (obs *Observer) WithTraceID(traceIDs ...string) *Observer
```

Each time a new request is generated, a new TraceID is **added**. For example:
* An end-user sent a request with TraceID `A` to service #1 to create a job.
* Service #1 received the request and sent requests services #2 and #3. The requests has TraceIDs: `A,B` and `A,C`.
* Service #3 received the request and sent a request to service #4. The TraceID is `A,C,D`.
* An end-user send a request with TraceID `E` to service #1 to cancel the job.
* Service #1 sent to service #3 a request with TraceID `E,F`.
* Service #3 sent to service #4 a request with TraceID `E,F,G`.
* Somewhere in service #4 among other logs there could be logs with TraceID `A,C,D,E,F,G`.

In log storage TraceIDs are supposed to be stored as a set of tags.

### Part 2.4: Observability API: in total

```go
package obs

type Tool interface {
	WithValue(key string, value interface{})
	WithFields(fields Fields)
	WithMap(map[string]interface{})
	WithStruct(interface{})
	Fields() Fields
}

type ToolEntry struct {
	Key ToolKey
	Tool Tool
}

type Tools []ToolEntry

func (tools Tools) Get(key ToolKey) Tool {
	for _, toolEntry := range tools {
		if toolEntry.Key == key {
			return toolEntry.Tool
		}
	}
	return false
}

type Observer struct { ... }

func (obs *Observer) WithValue(key string, value interface{}) *Observer { ... }
func (obs *Observer) WithFields(fields Fields) *Observer { ... }
func (obs *Observer) WithMap(map[string]interface{}) *Observer { ... }
func (obs *Observer) WithStruct(interface{}) *Observer { ... }
func (obs *Observer) Fields() Fields { ... }
func (obs *Observer) WithTools(tools ...Tool) { ... }
func (obs *Observer) Tools() Tools
func (obs *Observer) TraceIDs() []string
func (obs *Observer) WithTraceID(traceIDs ...string) *Observer
```

```go
package obscontext

import "context"

type ctxKey string
const ctxKeyObserver = ctxKey("observer")

func WithObs(ctx context.Context, obs *Observer) context.Context {
	return context.WithValue(ctx, ctxKeyObserver, obs)
}
func Obs(ctx context.Context) *Observer {
	observer := ctx.Value(ctxKeyObserver)
	if obs == nil {
		return obs.Default()
	}
	return observer.(*Observer)
}

func WithValue(ctx context.Context, key string, value interface{}) context.Context {
	obs := ctx.Value(ctxKeyObserver)
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, obs.WithValue(key, value))
}
func WithFields(ctx context.Context, fields Fields) context.Context { ... }
func WithMap(ctx context.Context, map[string]interface{}) context.Context { ... }
func WithStruct(ctx context.Context, interface{}) context.Context { ... }
```

Example of initializing with a context:
```go
ctx = obs.WithObs(ctx, obs.NewObserver(
	obsprom.New(prometheusMetrics),
	obszap.New(zapLogger),
	obszipkin.New(zipkinTracer),
	obssentry.New(sentryHandler),
))
```

Example of usage with a context:
```go
func myFunc(ctx context.Context, [...]) {
	ctx = obs.WithValue(ctx, "user_id", user.ID())
	defer tracer.FromCtx(ctx).StartSpan("fixing user info").Finish()

	[...]

	logger.FromCtx(ctx).Debug(user)

	[...]

	logger.FromCtx(ctx).Debugf("user is assigned into %d groups", len(user.Groups()))
	for _, group := range user.Groups() {
		go func(user *User, group *Group) {
			defer errmon.FromCtx(ctx).Recover()

			ctx = obs.WithValue(ctx, "group_id", group.ID())
			[...]
			anotherFunc(ctx)
		}(user, group)
	}
}
```

Example of initializing outside of a context:
```go
obs := obs.NewObserver(
	obsprom.New(prometheusMetrics),
	obszap.New(zapLogger),
	obszipkin.New(zipkinTracer),
	obssentry.New(sentryHandler),
))
```

Example of usage outside of a context:
```go
func myFunc(ctx context.Context, obs *obs.Observer, [...]) {
	obs = obs.WithValue("user_id", user.ID())
	defer tracer.FromObs(obs).StartSpan("fixing user info").Finish()

	[...]

	logger.FromObs(obs).Debug(user)

	[...]

	logger.FromObs(obs).Debugf("user is assigned into %d groups", len(user.Groups()))
	for _, group := range user.Groups() {
		go func(user *User, group *Group) {
			defer errmon.FromObs(obs).Recover()

			obs = obs.WithValue("group_id", group.ID())
			[...]
			anotherFunc(ctx, obs)
		}(user, group)
	}
}
```

### Part 3: Q&A

**How to flush and close a `Logger` atomically, preventing it from further writing?**

This is not a generic requirement, but a quite case-specific. It is not easy to find at least one Go project where logger is being closed. And if we will extend the generic interface with all case-specific methods, then it will grow indefinitely. For example there could be extract structured values already set to a `Logger`, but we also do not add it to the generic interface by the same reason.

If a project will require atomic Flush&Close, then they have multiple ways to solve this:

If they need `Close()` only in the root function then they can just use it directly:
```go
myLogger := NewMyLogger() // of a concrete type with method `Close`.
defer myLogger.Close()

myFunc(myLogger) // here it is boxed to an interface without method `Close`
```

Or they can have a close function separately:
```go
myLogger := NewMyLogger() // of a concrete type with method `Close`.
myFunc(myLogger, myLogger.Close) // here it is boxed to an interface without method `Close`
```

They can apply type-cast:

```go
myLogger.(io.Closer).Close()
```

Or they can just extend `Logger` interface:
```go
type Logger interface {
	logger.Logger
	io.Closer
}
```
and `Logger` implementation:
```go
type Logger struct {
	*obszap.Logger
}

func (l *Logger) Close() error {
	return l.Logger.Backend.Close()
}
```

Also if a `Logger` is being used as a singleton then it is possible to just use `sync/atomic` to atomically replace the logger with a dummy logger and flush the old one.

**Does it makes sense to do a generic `Logger` is it by design do not cover all possible use cases? May be it is better to use specific Loggers in each specific case?**

Generic logger allows different development teams to collaborate. It is similar to interface `io.ReadWriteCloser` which do not cover all possible use cases for IO objects. But it covers the part which is shared among them which makes such objects interchangable.

Having an own logger in each project is an inexcusable waste of time of talented engineers to re-invent the same wheel over and over. If there will be popular a generic `Logger` which covers the most of the cases, and the rest could be covered by for example type-assertion then it will provide an opportunity to re-use the same `Logger` code among the most of the projects.

**The approach with functions `FromObs` and `FromCtx` should negatively affect performance. What is supposed to do in high-performance applications?**

* It is expected that this kind of performance overhead might be essential only for metrics, because these functions are much less expensive than the log writing or values interface-boxing (for methods like `Debugf`).
* If there is a specific high-performance piece of code where such overhead may consume measurable amount of resources, then it is possible to extract a tool once and use it multiple times:

```go
func mySlowFunc(ctx context.Context) {
	[...]
	logger := logger.FromCtx(ctx)
	badUsersMetric := metric.FromCtx(ctx).Count("users_bad")
	for _, user := range billionsOfUsers {
		fastFunc(user, logger, badUsersMetric)
	}
}
```

It is also possible to pre-type-cast to make methods a little-bit faster:
```go
func mySlowFunc(ctx context.Context) {
	[...]
	logger := logger.FromCtx(ctx).(*obszap.Logger)
	badUsersMetric := metric.FromCtx(ctx).Count("users_bad").(*obsprom.Counter)
	for _, user := range billionsOfUsers {
		fastFunc(user, logger, badUsersMetric)
	}
}
```

Similar approach is used in official prometheus client package: they require to have prepared instances of metric families.
