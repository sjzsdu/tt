import { useCallback, useState } from 'react';
import { Button, Input, Popconfirm, Select, Space, Tag, message } from 'antd';
import { DeleteOutlined, LinkOutlined, PlusOutlined } from '@ant-design/icons';
import type { Issue, Dependency } from '../types';
import { api } from '../api';

const DEP_TYPES = [
  { label: 'Blocks', value: 'blocks' },
  { label: 'Related', value: 'related' },
  { label: 'Parent-Child', value: 'parent-child' },
];

type Props = {
  issue: Issue;
  issues: Issue[];
  readOnly?: boolean;
  onRefresh: () => void;
};

export function DependencyEditor({ issue, issues, readOnly, onRefresh }: Props) {
  const [adding, setAdding] = useState(false);
  const [depTarget, setDepTarget] = useState('');
  const [depType, setDepType] = useState('blocks');
  const [loading, setLoading] = useState(false);

  const deps = issue.dependencies || [];

  const handleAdd = useCallback(async () => {
    if (!depTarget.trim()) {
      message.warning('Enter a dependency target');
      return;
    }
    setLoading(true);
    try {
      await api.addDependency(issue.id, {
        depends_on_id: depTarget.trim(),
        type: depType,
      });
      message.success(`Added dependency: ${issue.id} → ${depTarget}`);
      setAdding(false);
      setDepTarget('');
      onRefresh();
    } catch (e) {
      message.error(e instanceof Error ? e.message : 'Failed to add dependency');
    } finally {
      setLoading(false);
    }
  }, [depTarget, depType, issue.id, onRefresh]);

  const handleRemove = useCallback(async (dependsOnId: string) => {
    setLoading(true);
    try {
      await api.removeDependency(issue.id, dependsOnId);
      message.success(`Removed dependency: ${issue.id} → ${dependsOnId}`);
      onRefresh();
    } catch (e) {
      message.error(e instanceof Error ? e.message : 'Failed to remove dependency');
    } finally {
      setLoading(false);
    }
  }, [issue.id, onRefresh]);

  // Available issues to depend on (exclude self)
  const availableTargets = issues.filter((i) => i.id !== issue.id);

  return (
    <div style={{ padding: '0 16px 16px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Space size={4}>
          <LinkOutlined />
          <strong>Dependencies</strong>
          <Tag>{deps.length}</Tag>
        </Space>
        {!readOnly && (
          <Button
            type="link"
            size="small"
            icon={<PlusOutlined />}
            onClick={() => setAdding(!adding)}
          >
            {adding ? 'Cancel' : 'Add'}
          </Button>
        )}
      </div>

      {adding && (
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <Select
            value={depType}
            onChange={setDepType}
            options={DEP_TYPES}
            style={{ width: 100 }}
            size="small"
          />
          <Select
            showSearch
            placeholder="Target issue ID"
            value={depTarget || undefined}
            onChange={setDepTarget}
            style={{ flex: 1 }}
            size="small"
            options={availableTargets.map((i) => ({
              label: `${i.id} - ${i.title}`,
              value: i.id,
            }))}
            filterOption={(input, option) =>
              (option?.label as string || '').toLowerCase().includes(input.toLowerCase())
            }
          />
          <Button
            type="primary"
            size="small"
            onClick={handleAdd}
            loading={loading}
          >
            Add
          </Button>
        </div>
      )}

      {deps.length === 0 ? (
        <div style={{ color: '#94a3b8', fontSize: 13, padding: '8px 0' }}>
          No dependencies
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          {deps.map((dep, i) => {
            const isSource = dep.issue_id === issue.id;
            const targetId = isSource ? dep.depends_on_id : dep.issue_id;
            const direction = isSource ? 'depends on' : 'blocked by';
            return (
              <div
                key={i}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '4px 8px',
                  borderRadius: 6,
                  background: 'rgba(148, 163, 184, 0.08)',
                }}
              >
                <Space size={4}>
                  <Tag color={dep.type === 'blocks' ? 'red' : 'default'} style={{ fontSize: 11 }}>
                    {dep.type}
                  </Tag>
                  <span style={{ fontSize: 12 }}>{direction}</span>
                  <code style={{ fontSize: 12 }}>{targetId}</code>
                </Space>
                {!readOnly && (
                  <Popconfirm
                    title="Remove this dependency?"
                    onConfirm={() => handleRemove(targetId)}
                    okText="Remove"
                    cancelText="Cancel"
                    okButtonProps={{ danger: true }}
                  >
                    <Button
                      type="text"
                      size="small"
                      danger
                      icon={<DeleteOutlined />}
                      loading={loading}
                    />
                  </Popconfirm>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
