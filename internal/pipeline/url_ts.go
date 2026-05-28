package pipeline

// @ts namespace global
// @doc
// The url module provides utilities for URL resolution and parsing.
// It exposes the WHATWG URL Standard-compliant URL and URLSearchParams classes
// as well as the domainToASCII and domainToUnicode helper functions.

// ---------------------------------------------------------------------------
// interface: URLSearchParamsIterator<T>
// ---------------------------------------------------------------------------

// @ts interface global.URLSearchParamsIterator<T>
// @doc An iterator over URLSearchParams entries, keys, or values.
// @extends Iterator<T, BuiltinIteratorReturn, unknown>
// @field [Symbol.iterator](): URLSearchParamsIterator<T> - Returns this iterator.

// ---------------------------------------------------------------------------
// class: URL
// ---------------------------------------------------------------------------

// @ts class global.URL
// @doc
// Browser-compatible URL class implemented by following the WHATWG URL Standard.
// All properties are implemented as getters and setters on the class prototype.
// Using the delete keyword on any property has no effect but still returns true.

// @ts constructor global.URL.new
// @doc Create a new URL object.
// @param input - string | { toString: () => string } - The URL string to parse.
// @param base? - string | URL - Optional base URL used to resolve relative input URLs.

// @ts field global.URL.hash
// @type string
// @doc
// Gets and sets the fragment portion of the URL (the part after #).
// Invalid URL characters in the assigned value are percent-encoded.

// @ts field global.URL.host
// @type string
// @doc
// Gets and sets the host portion of the URL, including the port.
// Invalid host values assigned to the property are ignored.

// @ts field global.URL.hostname
// @type string
// @doc
// Gets and sets the host name portion of the URL without the port.
// Invalid host name values assigned to the property are ignored.

// @ts field global.URL.href
// @type string
// @doc
// Gets and sets the serialized URL. Getting this value is equivalent to
// calling toString(). Setting it to an invalid URL throws a TypeError.

// @ts field global.URL.origin
// @readonly
// @type string
// @doc Gets the read-only serialization of the URL's origin.

// @ts field global.URL.password
// @type string
// @doc
// Gets and sets the password portion of the URL.
// Invalid URL characters in the assigned value are percent-encoded.

// @ts field global.URL.pathname
// @type string
// @doc
// Gets and sets the path portion of the URL.
// Invalid URL characters in the assigned value are percent-encoded.

// @ts field global.URL.port
// @type string
// @doc
// Gets and sets the port portion of the URL as a string.
// Values outside the range 0–65535 are ignored. Setting to the protocol's
// default port results in an empty string. Non-integer numbers are truncated.

// @ts field global.URL.protocol
// @type string
// @doc
// Gets and sets the protocol portion of the URL (including the trailing colon).
// Invalid protocol values are ignored.

// @ts field global.URL.search
// @type string
// @doc
// Gets and sets the serialized query portion of the URL.
// Invalid URL characters in the assigned value are percent-encoded.

// @ts field global.URL.searchParams
// @readonly
// @type URLSearchParams
// @doc
// Gets the URLSearchParams object representing the query parameters of the URL.
// This property is read-only but the returned object can mutate the URL instance.
// Use care when mixing the search setter and searchParams mutators, as they apply
// different percent-encoding rules.

// @ts field global.URL.username
// @type string
// @doc
// Gets and sets the username portion of the URL.
// Invalid URL characters in the assigned value are percent-encoded.

// @ts method global.URL.toString
// @doc
// Returns the serialized URL. Equivalent to reading the href property or toJSON().
// @returns string - The full serialized URL string.

// @ts method global.URL.toJSON
// @doc
// Returns the serialized URL. Equivalent to href and toString().
// Automatically called when the object is serialized with JSON.stringify().
// @returns string - The full serialized URL string.

// ---------------------------------------------------------------------------
// class: URLSearchParams
// ---------------------------------------------------------------------------

// @ts class global.URLSearchParams
// @doc
// The URLSearchParams API provides read and write access to the query of a URL.
// It can also be used standalone. The class is also available on the global object.
// @implements Iterable<[string, string]>

// @ts constructor global.URLSearchParams.new
// @doc Create a new URLSearchParams object.
// @param init? - URLSearchParams | string | Record<string, string | readonly string[]> | Iterable<[string, string]> | ReadonlyArray<[string, string]> - Optional initial data.

// @ts method global.URLSearchParams.append
// @doc Appends a new name-value pair to the query string.
// @param name - string - The parameter name.
// @param value - string - The parameter value.
// @returns void

// @ts method global.URLSearchParams.delete
// @doc
// Removes name-value pairs from the query string.
// If value is provided, only pairs matching both name and value are removed.
// If value is omitted, all pairs whose name matches are removed.
// @param name - string - The parameter name to remove.
// @param value? - string - Optional specific value to match.
// @returns void

// @ts method global.URLSearchParams.entries
// @doc
// Returns an ES6 Iterator over each name-value pair as a two-element [name, value] array.
// Alias for urlSearchParams[Symbol.iterator]().
// @returns URLSearchParamsIterator<[string, string]> - Iterator of [name, value] tuples.

// @ts method global.URLSearchParams.forEach
// @doc
// Iterates over each name-value pair in the query and invokes the given function.
// @param fn - (value: string, name: string, searchParams: URLSearchParams) => void - Callback invoked for each name-value pair.
// @param thisArg? - any - Value to use as this when calling fn.
// @returns void

// @ts method global.URLSearchParams.get
// @doc
// Returns the value of the first name-value pair whose name matches.
// Returns null if no such pair exists.
// @param name - string - The parameter name to look up.
// @returns string | null - The first matching value or null.

// @ts method global.URLSearchParams.getAll
// @doc
// Returns all values of name-value pairs whose name matches as an array.
// Returns an empty array if no such pairs exist.
// @param name - string - The parameter name to look up.
// @returns string[] - Array of all matching values.

// @ts method global.URLSearchParams.has
// @doc
// Checks whether the URLSearchParams object contains key-value pair(s).
// If value is provided, returns true only when both name and value match.
// If value is omitted, returns true if at least one pair with that name exists.
// @param name - string - The parameter name to check.
// @param value? - string - Optional specific value to check.
// @returns boolean - Whether a matching pair exists.

// @ts method global.URLSearchParams.keys
// @doc Returns an ES6 Iterator over the names of each name-value pair.
// @returns URLSearchParamsIterator<string> - Iterator of parameter names.

// @ts method global.URLSearchParams.set
// @doc
// Sets the value associated with name to value.
// If pre-existing pairs share that name, the first is updated and the rest are removed.
// If no pair exists, a new one is appended.
// @param name - string - The parameter name.
// @param value - string - The parameter value.
// @returns void

// @ts field global.URLSearchParams.size
// @readonly
// @type number
// @doc The total number of parameter entries.

// @ts method global.URLSearchParams.sort
// @doc
// Sorts all existing name-value pairs in-place by their names using a stable
// sorting algorithm, so relative order between pairs with the same name is preserved.
// Useful for increasing cache hits.
// @returns void

// @ts method global.URLSearchParams.toString
// @doc
// Returns the search parameters serialized as a string with characters
// percent-encoded where necessary.
// @returns string - The serialized query string.

// @ts method global.URLSearchParams.values
// @doc Returns an ES6 Iterator over the values of each name-value pair.
// @returns URLSearchParamsIterator<string> - Iterator of parameter values.

// @ts method global.URLSearchParams.symbolIterator
// @sig [Symbol.iterator](): URLSearchParamsIterator<[string, string]>

// ---------------------------------------------------------------------------
// module-level functions
// ---------------------------------------------------------------------------

// @ts function global.domainToASCII
// @doc
// Returns the Punycode ASCII serialization of the domain.
// Returns an empty string if the domain is invalid.
// Performs the inverse operation to domainToUnicode.
// @param domain - string - The Unicode domain name to convert.
// @returns string - The ASCII (Punycode) serialization, or an empty string.

// @ts function global.domainToUnicode
// @doc
// Returns the Unicode serialization of the domain.
// Returns an empty string if the domain is invalid.
// Performs the inverse operation to domainToASCII.
// @param domain - string - The Punycode domain name to convert.
// @returns string - The Unicode serialization, or an empty string.
