import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { Container } from 'react-bootstrap';
import 'bootstrap/dist/css/bootstrap.min.css';
import './App.css';

// Components
import Header from './components/Header';
import Dashboard from './pages/Dashboard';
import BlockchainViewer from './pages/BlockchainViewer';
import ShardDetails from './pages/ShardDetails';
import BlockDetails from './pages/BlockDetails';
import TransactionPool from './pages/TransactionPool';
import NetworkStats from './pages/NetworkStats';

function App() {
  return (
    <Router>
      <div className="App">
        <Header />
        <Container fluid className="mt-4">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/blockchain" element={<BlockchainViewer />} />
            <Route path="/shard/:shardId" element={<ShardDetails />} />
            <Route path="/block/:shardId/:blockHash" element={<BlockDetails />} />
            <Route path="/transactions" element={<TransactionPool />} />
            <Route path="/network" element={<NetworkStats />} />
          </Routes>
        </Container>
      </div>
    </Router>
  );
}

export default App;
