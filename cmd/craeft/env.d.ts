export {};
declare global {
  /**
   * Context passed into the `index` function.
   *
   */
  type iCtx = {
    /**
     * Variables set in the config.yaml file. Custom key/value mappings loaded from `config.yaml`. Values may include environment‑variable references (e.g. `$HOME/docs`), which will be expanded at runtime.
     */
    vars: Record<string, unknown>
  };
  /**
   * Context passed into the `transform` function.
   *
   */
  type tCtx = iCtx & {
    index: Record<string, unknown>
  };
}
