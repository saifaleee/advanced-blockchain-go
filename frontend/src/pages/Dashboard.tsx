import React, { useState, useEffect } from 'react';
import { Row, Col, Card, ListGroup, Alert } from 'react-bootstrap';
import { Link } from 'react-router-dom';
import ApiService, { BlockchainData } from '../services/api';
import { Chart as ChartJS, ArcElement, Tooltip, Legend, CategoryScale, LinearScale, BarElement, Title } from 'chart.js';
import { Pie, Bar } from 'react-chartjs-2';

// Register ChartJS components
ChartJS.register(ArcElement, Tooltip, Legend, CategoryScale, LinearScale, BarElement, Title);

const Dashboard: React.FC = () => {
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [blockchainData, setBlockchainData] = useState<BlockchainData | null>(null);
  const [refreshInterval, setRefreshInterval] = useState<NodeJS.Timeout | null>(null);

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
    // Load data initially
    loadData();

    // Set up refresh interval (every 10 seconds)
    const interval = setInterval(loadData, 10000);
    setRefreshInterval(interval);

    // Clean up interval on component unmount
    return () => {
      if (refreshInterval) {
        clearInterval(refreshInterval);
      }
    };
  }, []);

  // Prepare chart data for transactions by shard
  const getTransactionsByShardChartData = () => {
    if (!blockchainData) return null;

    const labels = blockchainData.shards.map(shard => `Shard ${shard.id}`);
    const txCounts = blockchainData.shards.map(shard => {
      return shard.blocks.reduce((total, block) => total + block.transactions.length, 0);
    });

    return {
      labels,
      datasets: [
        {
          label: 'Transactions',
          data: txCounts,
          backgroundColor: [
            'rgba(255, 99, 132, 0.6)',
            'rgba(54, 162, 235, 0.6)',
            'rgba(255, 206, 86, 0.6)',
            'rgba(75, 192, 192, 0.6)',
            'rgba(153, 102, 255, 0.6)',
          ],
          borderWidth: 1,
        },
      ],
    };
  };

  // Prepare chart data for blocks by shard
  const getBlocksByShardChartData = () => {
    if (!blockchainData) return null;

    const labels = blockchainData.shards.map(shard => `Shard ${shard.id}`);
    const blockCounts = blockchainData.shards.map(shard => shard.blocks.length);

    return {
      labels,
      datasets: [
        {
          label: 'Blocks',
          data: blockCounts,
          backgroundColor: 'rgba(54, 162, 235, 0.6)',
          borderWidth: 1,
        },
      ],
    };
  };

  return (
    <div className="dashboard">
      <h1 className="mb-4">Blockchain Dashboard</h1>
      
      {loading && !blockchainData && <Alert variant="info">Loading blockchain data...</Alert>}
      {error && <Alert variant="danger">{error}</Alert>}
      
      {blockchainData && (
        <>
          <Row className="mb-4">
            <Col md={3}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Total Blocks</Card.Title>
                  <div className="display-4 text-center">{blockchainData.metrics.totalBlocks}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={3}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Total Transactions</Card.Title>
                  <div className="display-4 text-center">{blockchainData.metrics.totalTransactions}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={3}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Cross-Shard Txs</Card.Title>
                  <div className="display-4 text-center">{blockchainData.metrics.totalCrossShardTxs}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={3}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Active Shards</Card.Title>
                  <div className="display-4 text-center">{blockchainData.shards.length}</div>
                </Card.Body>
              </Card>
            </Col>
          </Row>

          <Row className="mb-4">
            <Col md={6}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Transactions by Shard</Card.Title>
                  {getTransactionsByShardChartData() && <Pie data={getTransactionsByShardChartData()!} />}
                </Card.Body>
              </Card>
            </Col>
            <Col md={6}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Blocks by Shard</Card.Title>
                  {getBlocksByShardChartData() && (
                    <Bar 
                      data={getBlocksByShardChartData()!} 
                      options={{
                        scales: {
                          y: {
                            beginAtZero: true,
                            ticks: { precision: 0 }
                          }
                        }
                      }}
                    />
                  )}
                </Card.Body>
              </Card>
            </Col>
          </Row>

          <Row className="mb-4">
            <Col md={6}>
              <Card className="h-100">
                <Card.Header>Latest Blocks</Card.Header>
                <ListGroup variant="flush">
                  {blockchainData.shards.flatMap(shard => 
                    shard.blocks.slice(-3).map(block => ({
                      ...block,
                      shardId: shard.id
                    }))
                  )
                  .sort((a, b) => b.timestamp - a.timestamp)
                  .slice(0, 5)
                  .map(block => (
                    <ListGroup.Item key={block.hash}>
                      <Link to={`/block/${block.shardId}/${block.hash}`}>
                        <strong>Block {block.height}</strong> on Shard {block.shardId}
                      </Link>
                      <br />
                      <small>Hash: {block.hash.substring(0, 16)}...</small>
                      <br />
                      <small>Transactions: {block.transactions.length}</small>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              </Card>
            </Col>
            <Col md={6}>
              <Card className="h-100">
                <Card.Header>Validator Status</Card.Header>
                <ListGroup variant="flush">
                  {blockchainData.validators.map(validator => (
                    <ListGroup.Item key={validator.id}>
                      <strong>{validator.id}</strong>
                      <span className={`badge ms-2 ${validator.isActive ? 'bg-success' : 'bg-danger'}`}>
                        {validator.isActive ? 'Active' : 'Inactive'}
                      </span>
                      <br />
                      <small>Reputation: {validator.reputation}</small>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              </Card>
            </Col>
          </Row>
        </>
      )}
    </div>
  );
};

export default Dashboard; 