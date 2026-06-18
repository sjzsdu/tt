import { Button, Space, Tooltip } from 'antd';
import { CopyOutlined, DownloadOutlined, FileTextOutlined } from '@ant-design/icons';

interface D2ToolbarProps {
  subtitle: string;
  onExportSvg: () => void;
  onExportPng: () => void;
  onCopy: () => void;
}

export function D2Toolbar({ subtitle, onExportSvg, onExportPng, onCopy }: D2ToolbarProps) {
  return (
    <div className="d2-toolbar">
      <div className="d2-toolbar-title">
        <strong>D2 diagram</strong>
        <span>{subtitle}</span>
      </div>
      <Space size={6} wrap>
        <Tooltip title="Export SVG">
          <Button size="small" icon={<FileTextOutlined />} onClick={onExportSvg} />
        </Tooltip>
        <Tooltip title="Export PNG">
          <Button size="small" icon={<DownloadOutlined />} onClick={onExportPng} />
        </Tooltip>
        <Tooltip title="Copy PNG">
          <Button size="small" icon={<CopyOutlined />} onClick={onCopy} />
        </Tooltip>
      </Space>
    </div>
  );
}
