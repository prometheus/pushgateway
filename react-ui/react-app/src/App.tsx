import React, { FC } from 'react';
import { Container } from 'reactstrap';

import './App.css';
import { Router, Redirect } from '@reach/router';
import PathPrefixProps from './types/PathPrefixProps';

const App: FC<PathPrefixProps> = ({ pathPrefix }) => {
  return (
    <>
      <h1>Pushgateway</h1>
      <Container fluid style={{ paddingTop: 70 }}>
        <Router basepath={`${pathPrefix}/new`}>
          <Redirect from="/" to={`${pathPrefix}/new/graph`} />

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
