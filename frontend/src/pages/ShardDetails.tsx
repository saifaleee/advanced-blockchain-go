import React, { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Card, ListGroup, Badge, Row, Col, Alert } from 'react-bootstrap';
import ApiService, { ShardData } from '../services/api';

const ShardDetails: React.FC = () => {
  const { shardId } = useParams<{ shardId: string }>();
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [shardData, setShardData] = useState<ShardData | null>(null);

  useEffect(() => {
    const loadData = async () => {
      if (!shardId) return;
      
      try {
        setLoading(true);
        const shard = await ApiService.getShardById(parseInt(shardId));
        if (shard) {
          setShardData(shard);
          setError(null);
        } else {
          setError(`Shard ${shardId} not found`);
        }
      } catch (err) {
        console.error(`Error loading shard ${shardId}:`, err);
        setError(`Failed to load shard ${shardId}`);
      } finally {
        setLoading(false);
      }
    };

    loadData();
    
    // Auto-refresh every 10 seconds
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [shardId]);

  if (loading && !shardData) {
    return <Alert variant="info">Loading shard data...</Alert>;
  }

  if (error) {
    return <Alert variant="danger">{error}</Alert>;
  }

  if (!shardData) {
    return <Alert variant="warning">No shard data available</Alert>;
  }

  return (
    <div className="shard-details">
      <h1 className="mb-4">Shard {shardData.id} Details</h1>
      
      <Row className="mb-4">
        <Col md={4}>
          <Card>
            <Card.Body>
              <Card.Title>Shard Information</Card.Title>
              <ListGroup variant="flush">
                <ListGroup.Item>
                  <strong>Shard ID:</strong> {shardData.id}
                </ListGroup.Item>
                <ListGroup.Item>
                  <strong>Blocks:</strong> {shardData.blocks.length}
                </ListGroup.Item>
                <ListGroup.Item>
                  <strong>Transaction Pool Size:</strong> {shardData.txPoolSize}
                </ListGroup.Item>
                <ListGroup.Item>
                  <strong>State Size:</strong> {shardData.stateSize}
                </ListGroup.Item>
              </ListGroup>
            </Card.Body>
          </Card>
        </Col>
      </Row>
      
      <Card>
        <Card.Header>Blocks in Shard {shardData.id}</Card.Header>
        <Card.Body>
          <Row>
            {shardData.blocks
              .sort((a, b) => b.height - a.height)
              .map(block => (
                <Col md={4} key={block.hash} className="mb-3">
                  <Card>
                    <Card.Body>
                      <Card.Title>Block {block.height}</Card.Title>
                      <Card.Text>
                        <strong>Hash:</strong> {block.hash.substring(0, 16)}...<br />
                        <strong>Prev Hash:</strong> {block.prevHash.substring(0, 16)}...<br />
                        <strong>Transactions:</strong> {block.transactions.length}<br />
                        <strong>Timestamp:</strong> {new Date(block.timestamp * 1000).toLocaleString()}<br />
                      </Card.Text>
                      <Link to={`/block/${shardData.id}/${block.hash}`} className="btn btn-sm btn-primary">
                        View Details
                      </Link>
                    </Card.Body>
                  </Card>
                </Col>
              ))
            }
          </Row>
        </Card.Body>
      </Card>
    </div>
  );
};

export default ShardDetails; 