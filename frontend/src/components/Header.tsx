import React from 'react';
import { Navbar, Nav, Container } from 'react-bootstrap';
import { Link, useLocation } from 'react-router-dom';

const Header: React.FC = () => {
  const location = useLocation();

  return (
    <Navbar bg="dark" variant="dark" expand="lg">
      <Container>
        <Navbar.Brand as={Link} to="/">
          Advanced Blockchain Go
        </Navbar.Brand>
        <Navbar.Toggle aria-controls="basic-navbar-nav" />
        <Navbar.Collapse id="basic-navbar-nav">
          <Nav className="me-auto">
            <Nav.Link as={Link} to="/" active={location.pathname === '/'}>
              Dashboard
            </Nav.Link>
            <Nav.Link as={Link} to="/blockchain" active={location.pathname === '/blockchain'}>
              Blockchain Viewer
            </Nav.Link>
            <Nav.Link as={Link} to="/transactions" active={location.pathname === '/transactions'}>
              Transaction Pool
            </Nav.Link>
            <Nav.Link as={Link} to="/network" active={location.pathname === '/network'}>
              Network Stats
            </Nav.Link>
          </Nav>
        </Navbar.Collapse>
      </Container>
    </Navbar>
  );
};

export default Header; 