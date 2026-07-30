import { useState } from 'react';
import { CloseOutlined, EditOutlined, LinkOutlined } from '@ant-design/icons';
import { Button, Descriptions, Empty, Segmented, Tag, Typography } from 'antd';
import type { Issue } from '../types';
import type { UpdateIssueRequest } from '../api';
import { IssueForm } from './IssueForm';
import { DependencyEditor } from './DependencyEditor';

const { Paragraph, Title } = Typography;

const STATUS_COLORS: Record<string, string> = {
  open: 'blue',
  in_progress: 'orange',
  blocked: 'red',
  closed: 'green',
  deferred: 'default',
};

type Props = {
  issue: Issue | null;
  issues: Issue[];
  onClose: () => void;
  onNavigate: (id: string) => void;
  onUpdate: (id: string, req: UpdateIssueRequest) => Promise<void>;
  onRefresh: () => void;
  readOnly?: boolean;
};

type Tab = 'details' | 'edit';

export function IssueDetail({ issue, issues, onClose, onNavigate, onUpdate, onRefresh, readOnly }: Props) {
  const [tab, setTab] = useState<Tab>('details');

  if (!issue) {
    return (
      <div style={{ padding: 20 }}>
        <Empty description="Select an issue to view details" />
      </div>
    );
  }

  const deps = issue.dependencies || [];

  return (
    <div>
      {/* Header */}
      <div style={{ padding: 16, borderBottom: '1px solid rgba(148, 163, 184, 0.2)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
          <div>
            <code style={{ fontSize: 12, color: '#94a3b8' }}>{issue.id}</code>
            <Title level={5} style={{ margin: '4px 0 0' }}>{issue.title}</Title>
          </div>
          <Button type="text" icon={<CloseOutlined />} onClick={onClose} size="small" />
        </div>
        {!readOnly && (
          <Segmented
            size="small"
            value={tab}
            onChange={(v) => setTab(v as Tab)}
            options={[
              { label: 'Details', value: 'details' },
              { label: <span><EditOutlined /> Edit</span>, value: 'edit' },
            ]}
          />
        )}
      </div>

      {/* Content */}
      {tab === 'details' ? (
        <div style={{ padding: 16 }}>
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="Status">
              <Tag color={STATUS_COLORS[issue.status]}>{issue.status.replace('_', ' ')}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Priority">P{issue.priority}</Descriptions.Item>
            {issue.issue_type && (
              <Descriptions.Item label="Type">
                <Tag>{issue.issue_type}</Tag>
              </Descriptions.Item>
            )}
            {issue.assignee && (
              <Descriptions.Item label="Assignee">{issue.assignee}</Descriptions.Item>
            )}
            {issue.labels && issue.labels.length > 0 && (
              <Descriptions.Item label="Labels">
                {issue.labels.map((l) => <Tag key={l} color="processing">{l}</Tag>)}
              </Descriptions.Item>
            )}
            {issue.created_at && (
              <Descriptions.Item label="Created">{new Date(issue.created_at).toLocaleDateString()}</Descriptions.Item>
            )}
            {issue.estimated_minutes && (
              <Descriptions.Item label="Estimate">{issue.estimated_minutes}min</Descriptions.Item>
            )}
          </Descriptions>

          {issue.description && (
            <div style={{ marginTop: 16 }}>
              <Title level={5}>Description</Title>
              <Paragraph style={{ fontSize: 13, whiteSpace: 'pre-wrap' }}>
                {issue.description}
              </Paragraph>
            </div>
          )}

          {issue.design && (
            <div style={{ marginTop: 16 }}>
              <Title level={5}>Design</Title>
              <Paragraph style={{ fontSize: 13, whiteSpace: 'pre-wrap' }}>
                {issue.design}
              </Paragraph>
            </div>
          )}

          {issue.acceptance_criteria && (
            <div style={{ marginTop: 16 }}>
              <Title level={5}>Acceptance Criteria</Title>
              <Paragraph style={{ fontSize: 13, whiteSpace: 'pre-wrap' }}>
                {issue.acceptance_criteria}
              </Paragraph>
            </div>
          )}

          {/* Dependencies */}
          <DependencyEditor
            issue={issue}
            issues={issues}
            readOnly={readOnly}
            onRefresh={onRefresh}
          />
        </div>
      ) : (
        <IssueForm
          mode="edit"
          issue={issue}
          onSubmit={(req) => onUpdate(issue.id, req)}
          readOnly={readOnly}
        />
      )}
    </div>
  );
}
