/** @type {import('@hey-api/openapi-ts').UserConfig} */
export default {
  input: '../api/openapi.yaml',
  output: 'src/api/generated',
  plugins: ['@hey-api/typescript'],
};
