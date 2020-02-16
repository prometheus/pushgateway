import React, { FC } from 'react';
import Navigation from './Navbar';

import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import PathPrefixProps from './types/PathPrefixProps';

const App: FC<PathPrefixProps> = ({ pathPrefix }) => {
  return (
    <>
      <Navigation pathPrefix={pathPrefix} />
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/metrics`} />

          {/*
            NOTE: Any route added here needs to also be added to the list of
            React-handled router paths ("reactRouterPaths") in /main.go.
          */}
        </Router>
      </Container>
    </>
  );
};

export default App;
