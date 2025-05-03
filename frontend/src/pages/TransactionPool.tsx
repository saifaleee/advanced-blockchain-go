import React, { useState, useEffect } from 'react';
import { Card, Table, Badge, Button, Alert } from 'react-bootstrap';
import { Link } from 'react-router-dom';
import ApiService, { BlockchainData } from '../services/api';

const TransactionPool: React.FC = () => {
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [blockchainData, setBlockchainData] = useState<BlockchainData | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const data = await ApiService.getBlockchainData();
      setBlockchainData(data);
      setError(null);
    } catch (err) {
      console.error('Failed to load blockchain data', err);
      setError('Failed to load blockchain data. Please check if the blockchain server is running.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    
    // Auto-refresh every 5 seconds
    const interval = setInterval(loadData, 5000);
    return () => clearInterval(interval);
  }, []);

  // Helper function to determine transaction type label
  const getTransactionTypeLabel = (type: number) => {
    switch (type) {
      case 0:
        return <Badge bg="primary">IntraShard</Badge>;
      case 1:
        return <Badge bg="success">CrossShard Init</Badge>;
      case 2:
        return <Badge bg="info">CrossShard Ack</Badge>;
      case 3:
        return <Badge bg="warning">CrossShard Commit</Badge>;
      default:
        return <Badge bg="secondary">Unknown</Badge>;
    }
  };

  // Calculate summary statistics
  const calculateStats = () => {
    if (!blockchainData) return { total: 0, intra: 0, cross: 0 };
    
    let totalTxs = 0;
    let intraTxs = 0;
    let crossTxs = 0;
    
    blockchainData.shards.forEach(shard => {
      totalTxs += shard.txPoolSize;
      // We don't have direct access to tx types in the pool, so this is just for display
      // In a real implementation, we'd get this data from the API
      intraTxs = Math.floor(totalTxs * 0.7); // Just for demo - assuming 70% are intra-shard
      crossTxs = totalTxs - intraTxs;
    });
    
    return { total: totalTxs, intra: intraTxs, cross: crossTxs };
  };

  const stats = calculateStats();

  return (
    <div className="transaction-pool">
      <h1 className="mb-4">Transaction Pool</h1>
      
      {loading && !blockchainData && <Alert variant="info">Loading transaction data...</Alert>}
      {error && <Alert variant="danger">{error}</Alert>}
      
      <Card className="mb-4">
        <Card.Body>
          <Card.Title>Transaction Pool Summary</Card.Title>
          <div className="d-flex justify-content-between">
            <div className="text-center p-3">
              <h5>Total Pending</h5>
              <div className="display-6">{stats.total}</div>
            </div>
            <div className="text-center p-3">
              <h5>Intra-Shard</h5>
              <div className="display-6">{stats.intra}</div>
            </div>
            <div className="text-center p-3">
              <h5>Cross-Shard</h5>
              <div className="display-6">{stats.cross}</div>
            </div>
            <div className="d-flex align-items-center">
              <Button variant="primary" onClick={loadData}>
                Refresh
              </Button>
            </div>
          </div>
        </Card.Body>
      </Card>
      
      {blockchainData && (
        <Card>
          <Card.Header>Pending Transactions by Shard</Card.Header>
          <Card.Body>
            {blockchainData.shards.map(shard => (
              <div key={shard.id} className="mb-4">
                <h5>
                  Shard {shard.id} 
                  <Badge bg="secondary" className="ms-2">{shard.txPoolSize} transactions</Badge>
                </h5>
                {shard.txPoolSize === 0 ? (
                  <Alert variant="info">No pending transactions in this shard</Alert>
                ) : (
                  <Alert variant="secondary">
                    Transaction pool data is summarized. In a production implementation, 
                    this would display the actual pending transactions from each shard's pool.
                  </Alert>
                )}
                {shard.txPoolSize > 0 && (
                  <Table striped bordered hover responsive>
                    <thead>
                      <tr>
                        <th>ID</th>
                        <th>Type</th>
                        <th>From Shard</th>
                        <th>To Shard</th>
                        <th>Data</th>
                        <th>Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {/* Example placeholder transaction - would be replaced with real data */}
                      <tr>
                        <td><code>example-tx-{shard.id}</code></td>
                        <td>{getTransactionTypeLabel(Math.floor(Math.random() * 4))}</td>
                        <td><Link to={`/shard/${shard.id}`}>{shard.id}</Link></td>
                        <td><Link to={`/shard/${shard.id}`}>{shard.id}</Link></td>
                        <td>Example transaction data</td>
                        <td><Badge bg="secondary">Pending</Badge></td>
                      </tr>
                    </tbody>
                  </Table>
                )}
              </div>
            ))}
          </Card.Body>
        </Card>
      )}
    </div>
  );
};

export default TransactionPool; 