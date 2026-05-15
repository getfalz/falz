/**
 * craeft-hotreload.js — WebSocket hot-reload client for the craeft dev server.
 *
 *   <script type="module">
 *     import { CraeftHotReload } from './craeft-hotreload.js';
 *     new CraeftHotReload().connect();
 *   </script>
 *
 * Advanced usage with custom callbacks:
 *
 *   <script type="module">
 *     import { CraeftHotReload } from './craeft-hotreload.js';
 *
 *     const hr = new CraeftHotReload({ reconnectDelay: 2000 });
 *
 *     hr.on('reload',   ()               => window.location.reload())
 *       .on('navigate', ({ url })         => { window.location.href = url; })
 *       .on('built',    ({ path })        => console.log('rebuilt:', path))
 *       .on('error',    ({ path, error }) => console.error(path, error))
 *       .on('open',     ()               => console.debug('connected'))
 *       .on('close',    ()               => console.debug('disconnected'));
 *
 *     hr.connect();
 *
 *     // Ask the server to force-rebuild a specific source file:
 *     hr.build('notes/2024-07-08.md');
 *
 *     hr.close(); // disconnect + stop reconnecting
 *   </script>
 */

/** @typedef {'reload'|'navigate'|'built'|'error'|'open'|'close'} EventName */

/**
 * @typedef {Object} CraeftHotReloadOptions
 * @property {string}  [url]            - WebSocket URL. Defaults to ws(s)://<host>/ws.
 * @property {number}  [reconnectDelay] - Milliseconds between reconnect attempts (default 1500).
 * @property {number}  [maxRetries]     - Maximum consecutive reconnect attempts (default Infinity).
 */

export class CraeftHotReload {
  /** @type {WebSocket|null} */
  #socket = null;

  /** @type {Map<EventName, Set<Function>>} */
  #handlers = new Map();

  /** @type {boolean} */
  #intentionallyClosed = false;

  /** @type {number} */
  #retries = 0;

  /** @type {ReturnType<typeof setTimeout>|null} */
  #reconnectTimer = null;

  /** @type {string} */
  #url;

  /** @type {number} */
  #reconnectDelay;

  /** @type {number} */
  #maxRetries;

  /**
   * @param {CraeftHotReloadOptions} [opts]
   */
  constructor(opts = {}) {
    const {
      url = CraeftHotReload.#defaultURL(),
      reconnectDelay = 1500,
      maxRetries = Infinity,
    } = opts;

    this.#url = url;
    this.#reconnectDelay = reconnectDelay;
    this.#maxRetries = maxRetries;
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Register a callback for a named event.
   *
   * Built-in server events:  'reload' | 'navigate' | 'built' | 'error'
   * Connection lifecycle:    'open' | 'close'
   *
   * Multiple handlers per event are supported; call `on` several times.
   *
   * @param {EventName} event
   * @param {Function}  handler
   * @returns {this}  (chainable)
   */
  on(event, handler) {
    if (typeof handler !== "function") {
      throw new TypeError(
        `CraeftHotReload.on: handler for "${event}" must be a function`,
      );
    }
    if (!this.#handlers.has(event)) {
      this.#handlers.set(event, new Set());
    }
    this.#handlers.get(event).add(handler);
    return this;
  }

  /**
   * Remove a previously registered callback. Omit `handler` to remove all
   * callbacks for that event.
   *
   * @param {EventName} event
   * @param {Function}  [handler]
   * @returns {this}
   */
  off(event, handler) {
    if (!handler) {
      this.#handlers.delete(event);
    } else {
      this.#handlers.get(event)?.delete(handler);
    }
    return this;
  }

  /**
   * Open the WebSocket connection (and enable automatic reconnect).
   * Safe to call multiple times – no-ops if already connected.
   *
   * @returns {this}
   */
  connect() {
    if (this.#socket && this.#socket.readyState <= WebSocket.OPEN) {
      return this;
    }
    this.#intentionallyClosed = false;
    this.#open();
    return this;
  }

  /**
   * Close the connection and disable automatic reconnect.
   */
  close() {
    this.#intentionallyClosed = true;
    clearTimeout(this.#reconnectTimer);
    this.#socket?.close(1000, "client closed");
    this.#socket = null;
  }

  /**
   * Request the server to force-build a source file by its project-relative path.
   *
   * The server will reply with a 'built' or 'error' event directed to this
   * client only.
   *
   * @param {string} sourcePath - Project-relative source path, e.g. "notes/foo.md"
   */
  build(sourcePath) {
    this.#send({ type: "build", path: sourcePath });
  }

  /** True if the socket is currently open. */
  get connected() {
    return this.#socket?.readyState === WebSocket.OPEN;
  }

  // ---------------------------------------------------------------------------
  // Internal
  // ---------------------------------------------------------------------------

  /** @returns {string} */
  static #defaultURL() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    return `${proto}://${location.host}/ws`;
  }

  #open() {
    const socket = new WebSocket(this.#url);
    this.#socket = socket;

    socket.addEventListener("open", () => {
      this.#retries = 0;
      this.#emit("open", null);
    });

    socket.addEventListener("message", (evt) => {
      let data;
      try {
        data = JSON.parse(evt.data);
      } catch {
        console.warn("[craeft-ws] received non-JSON message:", evt.data);
        return;
      }
      if (data && typeof data.type === "string") {
        this.#emit(data.type, data);
      }
    });

    socket.addEventListener("close", (evt) => {
      this.#emit("close", { code: evt.code, reason: evt.reason });
      if (!this.#intentionallyClosed) {
        this.#scheduleReconnect();
      }
    });

    socket.addEventListener("error", () => {
      // The 'close' event always follows an error, so we only log here.
      console.warn("[craeft-ws] connection error");
    });
  }

  #scheduleReconnect() {
    if (this.#retries >= this.#maxRetries) {
      console.warn(
        `[craeft-ws] max retries (${this.#maxRetries}) reached; giving up`,
      );
      return;
    }

    const delay = Math.min(this.#reconnectDelay * 2 ** this.#retries, 30_000);
    this.#retries++;

    console.info(
      `[craeft-ws] reconnecting in ${delay}ms (attempt ${this.#retries})`,
    );
    this.#reconnectTimer = setTimeout(() => this.#open(), delay);
  }

  /**
   * @param {string}  event
   * @param {any}     payload
   */
  #emit(event, payload) {
    const set = this.#handlers.get(event);
    if (!set || set.size === 0) return;
    for (const fn of set) {
      try {
        fn(payload);
      } catch (err) {
        console.error(`[craeft-ws] handler for "${event}" threw:`, err);
      }
    }
  }

  /**
   * @param {object} msg
   */
  #send(msg) {
    if (!this.connected) {
      console.warn(
        "[craeft-ws] send called while disconnected; message dropped:",
        msg,
      );
      return;
    }
    this.#socket.send(JSON.stringify(msg));
  }
}
