import { Empty, Table, Tag, Tooltip } from 'antd';
import type { Issue } from '../types';

const PRIORITY_COLORS: Record<number, string> = {
  0: 'red',
  1: 'volcano',
  2: 'orange',
  3: 'blue',
  4: 'default',
};

const PRIORITY_LABELS: Record<number, string> = {
  0: 'P0 Critical',
  1: 'P1 High',
  2: 'P2 Medium',
  3: 'P3 Low',
  4: 'P4 None',
};

const STATUS_COLORS: Record<string, string> = {
  open: 'blue',
  in_progress: 'orange',
  blocked: 'red',
  closed: 'green',
  deferred: 'default',
};

type Props = {
  issues: Issue[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  statusIcons: Record<string, React.ReactNode>;
};

export function IssueList({ issues, selectedId, onSelect, statusIcons }: Props) {
  if (issues.length === 0) {
    return (
      <div className="beads-empty">
        <Empty description="No issues found" />
      </div>
    );
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 120,
      render: (id: string) => <code style={{ fontSize: 12 }}>{id}</code>,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag icon={statusIcons[status]} color={STATUS_COLORS[status]}>
          {status.replace('_', ' ')}
        </Tag>
      ),
    },
    {
      title: 'Priority',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
      render: (p: number) => (
        <Tag color={PRIORITY_COLORS[p]}>{PRIORITY_LABELS[p] || `P${p}`}</Tag>
      ),
    },
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      ellipsis: true,
      render: (title: string, record: Issue) => (
        <Tooltip title={title}>
          <span style={{ cursor: 'pointer' }} onClick={() => onSelect(record.id)}>
            {title}
          </span>
        </Tooltip>
      ),
    },
    {
      title: 'Labels',
      dataIndex: 'labels',
      key: 'labels',
      width: 200,
      render: (labels: string[]) => labels?.map((l) => (
        <Tag key={l} color="processing" style={{ fontSize: 11 }}>{l}</Tag>
      )),
    },
    {
      title: 'Type',
      dataIndex: 'issue_type',
      key: 'issue_type',
      width: 80,
      render: (t: string) => t ? <Tag>{t}</Tag> : null,
    },
  ];

  return (
    <Table
      dataSource={issues}
      columns={columns}
      rowKey="id"
      size="small"
      pagination={{ pageSize: 50, showSizeChanger: true, pageSizeOptions: [20, 50, 100] }}
      onRow={(record) => ({
        onClick: () => onSelect(record.id),
        style: {
          cursor: 'pointer',
          background: record.id === selectedId ? 'rgba(99, 102, 241, 0.1)' : undefined,
        },
      })}
    />
  );
}
