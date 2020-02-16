import React from 'react';
import ReactDOM from 'react-dom';
import './index.css';
import App from './App';

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_PATH_PREFIX: string;

let prefix = GLOBAL_PATH_PREFIX;

if (GLOBAL_PATH_PREFIX === 'PATH_PREFIX_PLACEHOLDER' || GLOBAL_PATH_PREFIX === '/') {
  // Either we are running the app outside of Prometheus, so the placeholder value in
  // the index.html didn't get replaced, or we have a '/' prefix, which we also need to
  // normalize to '' to make concatenations work (prefixes like '/foo/bar/' already get
  // their trailing slash stripped by Prometheus).
  prefix = '';
}

ReactDOM.render(<App pathPrefix={prefix} />, document.getElementById('root'));
