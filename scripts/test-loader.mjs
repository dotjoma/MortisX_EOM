export async function resolve(specifier, context, defaultResolve) {
  if (specifier.startsWith('./') || specifier.startsWith('../')) {
    try {
      return await defaultResolve(specifier, context);
    } catch {
      for (const ext of ['.ts', '.tsx', '.js', '/index.ts', '/index.js']) {
        try {
          return await defaultResolve(specifier + ext, context);
        } catch {}
      }
    }
  }
  return defaultResolve(specifier, context);
}
