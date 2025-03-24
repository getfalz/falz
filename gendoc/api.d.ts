declare global {
/**
 * 
 * Renders a template using the provided variables.
 * 
 * @param template - The template string to render.
 * @param variables - A key-value map containing variables to be used in the template.
 * @returns A promise that resolves to the rendered string.
 * 
 **/
function Render(template: string, variables: Record<string, any>): Promise<string>;
}
export {};