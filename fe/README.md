# fe

The Vue 3 frontend for iMail.

```sh
yarn
```

### Compile and Hot-Reload for Development

```sh
yarn dev
```

### Compile and Minify for Production

```sh
yarn build
```

### Lint with [ESLint](https://eslint.org/)

```sh
yarn lint
```

### Browser Regression Tests

Tests the maximum height of the mail body, sandbox script blocking and cross-origin link navigation. Chrome or Chromium must be installed locally; the test tooling does not download a browser automatically.

```sh
yarn test
# When the browser is not in the default location:
CHROME_BIN=/path/to/chrome yarn test
```
