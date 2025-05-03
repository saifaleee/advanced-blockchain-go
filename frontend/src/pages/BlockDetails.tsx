import React, { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Card, ListGroup, Badge, Table, Alert } from 'react-bootstrap';
import ApiService, { BlockData } from '../services/api';

const BlockDetails: React.FC = () => {
  const { shardId, blockHash } = useParams<{ shardId: string; blockHash: string }>();
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [blockData, setBlockData] = useState<BlockData | null>(null);

  useEffect(() => {
    const loadData = async () => {
      if (!shardId || !blockHash) return;
      
      try {
        setLoading(true);
        const block = await ApiService.getBlockByHash(parseInt(shardId), blockHash);
        if (block) {
          setBlockData(block);
          setError(null);
        } else {
          setError(`Block ${blockHash} not found in shard ${shardId}`);
        }
      } catch (err) {
        console.error(`Error loading block ${blockHash}:`, err);
        setError(`Failed to load block ${blockHash}`);
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, [shardId, blockHash]);

  if (loading && !blockData) {
    return <Alert variant="info">Loading block data...</Alert>;
  }

  if (error) {
    return <Alert variant="danger">{error}</Alert>;
  }

  if (!blockData) {
    return <Alert variant="warning">No block data available</Alert>;
  }

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

  // Helper function to determine transaction status label
  const getTransactionStatusLabel = (status: number) => {
    switch (status) {
      case 0:
        return <Badge bg="secondary">Pending</Badge>;
      case 1:
        return <Badge bg="success">Confirmed</Badge>;
      case 2:
        return <Badge bg="danger">Failed</Badge>;
      default:
        return <Badge bg="light" text="dark">Unknown</Badge>;
    }
  };

  return (
    <div className="block-details">
      <h1 className="mb-4">Block Details</h1>
      
      <Card className="mb-4">
        <Card.Body>
          <Card.Title>Block Information</Card.Title>
          <ListGroup variant="flush">
            <ListGroup.Item>
              <strong>Block Height:</strong> {blockData.height}
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Shard ID:</strong> <Link to={`/shard/${blockData.shardId}`}>{blockData.shardId}</Link>
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Hash:</strong> <code>{blockData.hash}</code>
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Previous Hash:</strong> <code>{blockData.prevHash}</code>
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Timestamp:</strong> {new Date(blockData.timestamp * 1000).toLocaleString()}
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Nonce:</strong> {blockData.nonce}
            </ListGroup.Item>
            <ListGroup.Item>
              <strong>Transaction Count:</strong> {blockData.transactions.length}
            </ListGroup.Item>
          </ListGroup>
        </Card.Body>
      </Card>
      
      <Card>
        <Card.Header>Transactions</Card.Header>
        <Card.Body>
          {blockData.transactions.length === 0 ? (
            <Alert variant="info">No transactions in this block</Alert>
          ) : (
            <Table striped bordered hover responsive>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Type</th>
                  <th>Data</th>
                  <th>From Shard</th>
                  <th>To Shard</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {blockData.transactions.map((tx) => (
                  <tr key={tx.id}>
                    <td><code>{tx.id.substring(0, 8)}...</code></td>
                    <td>{getTransactionTypeLabel(tx.type)}</td>
                    <td>{tx.data}</td>
                    <td><Link to={`/shard/${tx.fromShard}`}>{tx.fromShard}</Link></td>
                    <td><Link to={`/shard/${tx.toShard}`}>{tx.toShard}</Link></td>
                    <td>{getTransactionStatusLabel(tx.status)}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card.Body>
      </Card>
    </div>
  );
};

export default BlockDetails; 