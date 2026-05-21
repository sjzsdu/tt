declare module '*.css';
declare module '*?url' {
  const url: string;
  export default url;
}
declare module '*.woff?url' {
  const url: string;
  export default url;
}
declare module '*.wasm?url' {
  const url: string;
  export default url;
}
declare module '@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff?url';
declare module '@resvg/resvg-wasm/index_bg.wasm?url';
