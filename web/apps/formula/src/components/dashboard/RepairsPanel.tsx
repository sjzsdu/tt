import { Alert, Button, Card, List, Space, Tag, Typography } from 'antd';
import type { FormulaRepairRecord } from '../../types';

const statusColor: Record<string, string> = {
  succeeded: 'success',
  exhausted: 'error',
  attempt_failed: 'warning',
  skipped_non_idempotent: 'default',
  confirmed: 'processing',
};

export function RepairsPanel({ repairs, onConfirm, busyKey }: { repairs: FormulaRepairRecord[]; onConfirm: (stepID: string, attempt: number) => Promise<void>; busyKey?: string }) {
  if (!repairs.length) return null;
  const sorted = [...repairs].sort((a, b) => {
    const left = a.recorded_at || '';
    const right = b.recorded_at || '';
    if (left !== right) return left < right ? 1 : -1;
    return (b.attempt || 0) - (a.attempt || 0);
  });
  return (
    <Card title="Repairs" className="run-overview-card">
      <List
        dataSource={sorted}
        renderItem={repair => {
          const key = `${repair.step_id}:${repair.attempt || 0}`;
          const confirmed = !!repair.confirmed_at || repair.confirmation_status === 'confirmed';
          return (
            <List.Item
              actions={confirmed || !repair.formula_update_hint ? [] : [
                <Button key="confirm" size="small" type="primary" ghost loading={busyKey === key} onClick={() => onConfirm(repair.step_id, repair.attempt || 0)}>
                  Confirm reviewed
                </Button>,
              ]}
            >
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space wrap>
                  <Typography.Text strong>{repair.step_id}</Typography.Text>
                  <Tag color={statusColor[repair.status || ''] || 'default'}>{repair.status || 'repair'}</Tag>
                  {repair.attempt ? <Tag>attempt {repair.attempt}</Tag> : null}
                  {confirmed ? <Tag color="processing">confirmed</Tag> : null}
                </Space>
                {repair.reason ? <Typography.Text>{repair.reason}</Typography.Text> : null}
                {repair.formula_update_hint ? <Alert type="info" showIcon message="Suggested formula update" description={repair.formula_update_hint} /> : null}
                {repair.next_attempt_hint ? <Typography.Text type="secondary">{repair.next_attempt_hint}</Typography.Text> : null}
                {repair.fixed_command?.length ? <Typography.Text code>{repair.fixed_command.join(' ')}</Typography.Text> : null}
                {repair.error ? <Typography.Text type="danger">{repair.error}</Typography.Text> : null}
              </Space>
            </List.Item>
          );
        }}
      />
    </Card>
  );
}
