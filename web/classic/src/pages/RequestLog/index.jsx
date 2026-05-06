/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Empty,
  Form,
  Space,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { IllustrationNoResult, IllustrationNoResultDark } from '@douyinfe/semi-illustrations';
import { API, showError, timestamp2string } from '../../helpers';
import { DATE_RANGE_PRESETS } from '../../constants/console.constants';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import { createCardProPagination } from '../../helpers/utils';
import { useIsMobile } from '../../hooks/common/useIsMobile';

const { Paragraph, Text } = Typography;

const DEFAULT_PAGE_SIZE = 20;

const getRangeTimestamps = (dateRange) => {
  if (!dateRange || dateRange.length !== 2) {
    return { startTimestamp: 0, endTimestamp: 0 };
  }
  return {
    startTimestamp: Math.floor(new Date(dateRange[0]).getTime() / 1000),
    endTimestamp: Math.floor(new Date(dateRange[1]).getTime() / 1000),
  };
};

const renderText = (value) => value || '-';
const RequestLogPage = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [logCount, setLogCount] = useState(0);

  const loadLogs = async (page = activePage, size = pageSize) => {
    setLoading(true);
    const values = formApiRef.current?.getValues() || {};
    const { startTimestamp, endTimestamp } = getRangeTimestamps(values.dateRange);
    const params = new URLSearchParams({
      p: String(page),
      page_size: String(size),
    });

    if (startTimestamp) params.set('start_timestamp', String(startTimestamp));
    if (endTimestamp) params.set('end_timestamp', String(endTimestamp));
    if (values.request_id) params.set('request_id', values.request_id);
    if (values.method) params.set('method', values.method);
    if (values.path) params.set('path', values.path);
    if (values.status_code) params.set('status_code', values.status_code);
    if (values.username) params.set('username', values.username);

    try {
      const res = await API.get(`/api/log/request?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setLogs(
          (data.items || []).map((item) => ({
            ...item,
            key: item.id,
          })),
        );
        setActivePage(data.page);
        setPageSize(data.page_size);
        setLogCount(data.total);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    loadLogs(1, DEFAULT_PAGE_SIZE);
  }, []);

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      {
        title: t('请求时间'),
        dataIndex: 'created_at',
        width: 160,
        render: (text) => (text ? timestamp2string(text) : '-'),
      },
      {
        title: 'Request ID',
        dataIndex: 'request_id',
        width: 160,
        render: (text) => {
          if (!text) return '-';
          return (
            <Paragraph copyable={{ content: text }}>{text}</Paragraph>
          );
        },
      },
      { title: t('用户名称'), dataIndex: 'username', width: 120, render: (text) => renderText(text) },
      {
        title: t('模型名称'),
        dataIndex: 'model_name',
        width: 140,
        render: (text) => renderText(text),
      },
      {
        title: t('方法'),
        dataIndex: 'method',
        width: 80,
        render: (text) => {
          const colorMap = { GET: 'green', POST: 'blue', PUT: 'orange', DELETE: 'red' };
          return <Tag color={colorMap[text] || 'grey'}>{text || '-'}</Tag>;
        },
      },
      {
        title: t('路径'),
        dataIndex: 'path',
        width: 200,
        ellipsis: true,
        render: (text) => renderText(text),
      },
      {
        title: t('状态码'),
        dataIndex: 'status_code',
        width: 90,
        render: (text) => {
          if (!text && text !== 0) return '-';
          const color = text < 300 ? 'green' : text < 400 ? 'blue' : text < 500 ? 'orange' : 'red';
          return <Tag color={color}>{text}</Tag>;
        },
      },
      {
        title: t('耗时'),
        dataIndex: 'duration_ms',
        width: 100,
        render: (text) => (text !== undefined && text !== null ? `${text}ms` : '-'),
      },
      {
        title: t('客户端 IP'),
        dataIndex: 'client_ip',
        width: 130,
        render: (text) => renderText(text),
      },
      { title: 'User Agent', dataIndex: 'user_agent', width: 150, ellipsis: true, render: (text) => renderText(text) },
    ],
    [t],
  );
  const formInitValues = {
    dateRange: undefined,
    request_id: '',
    method: '',
    path: '',
    status_code: '',
    username: '',
  };

  const searchArea = (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => (formApiRef.current = api)}
      onSubmit={() => loadLogs(1, pageSize)}
      allowEmpty
      autoComplete="off"
      layout="vertical"
      trigger="change"
      stopValidateWithError={false}
    >
      <div className="flex flex-col gap-2">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2">
          <div className="col-span-1 lg:col-span-2">
            <Form.DatePicker
              field="dateRange"
              className="w-full"
              type="dateTimeRange"
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size="small"
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>
          <Form.Input field="request_id" prefix={<IconSearch />} placeholder="Request ID" showClear pure size="small" />
          <Form.Input field="method" prefix={<IconSearch />} placeholder={t('方法')} showClear pure size="small" />
          <Form.Input field="path" prefix={<IconSearch />} placeholder={t('路径')} showClear pure size="small" />
          <Form.Input field="status_code" prefix={<IconSearch />} placeholder={t('状态码')} showClear pure size="small" />
          <Form.Input field="username" prefix={<IconSearch />} placeholder={t('用户名称')} showClear pure size="small" />
        </div>
        <div className="flex gap-2">
          <Button type="tertiary" htmlType="submit" loading={loading} size="small">
            {t('查询')}
          </Button>
          <Button
            type="tertiary"
            size="small"
            onClick={() => {
              formApiRef.current?.reset();
              loadLogs(1, DEFAULT_PAGE_SIZE);
            }}
          >
            {t('重置')}
          </Button>
        </div>
      </div>
    </Form>
  );
  return (
    <div className="mt-[60px] px-2">
      <CardPro
        type="type2"
        searchArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: activePage,
          pageSize,
          total: logCount,
          onPageChange: (page) => {
            setActivePage(page);
            loadLogs(page, pageSize);
          },
          onPageSizeChange: (size) => {
            setPageSize(size);
            setActivePage(1);
            loadLogs(1, size);
          },
          isMobile,
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={logs}
          rowKey="key"
          loading={loading}
          scroll={{ x: 'max-content' }}
          className="rounded-xl overflow-hidden"
          size="small"
          empty={
            <Empty
              image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
              darkModeImage={<IllustrationNoResultDark style={{ width: 150, height: 150 }} />}
              description={t('搜索无结果')}
              style={{ padding: 30 }}
            />
          }
        />
      </CardPro>
    </div>
  );
};

export default RequestLogPage;
