export {};
declare global {
  interface Console {
    /**
     * Logs the provided arguments to the console with info level.
     */
    log(...data: any[]): void
    /**
     * Logs the provided arguments to the console with info level.
     */
    info(...data: any[]): void
    /**
     * Logs the provided arguments to the console with warn level.
     */
    warn(...data: any[]): void
    /**
     * Logs the provided arguments to the console with error level.
     */
    error(...data: any[]): void
    /**
     * Dumps the provided arguments to the console with error level.
     */
    dump(...data: any[]): void
  }
  /**
   * Console provides a simple logging interface for JavaScript pipelines.
   *
   */
  export var console: Console
}
