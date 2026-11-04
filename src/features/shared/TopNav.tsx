import React from 'react';
import { Icon, IconName } from '../../components/Icon';

interface TopNavProps {
  activeNav: string;
  onSelectNav: (nav: string) => void;
}

const NAV_ITEMS: { id: string; icon: IconName }[] = [
  { id: 'Bots', icon: 'smart_toy' },
  { id: 'List', icon: 'format_list_bulleted' },
  { id: 'Executor', icon: 'code' },
  { id: 'Database', icon: 'database' },
  { id: 'Settings', icon: 'settings' },
];

export const TopNav: React.FC<TopNavProps> = ({ activeNav, onSelectNav }) => {
  return (
    <div className="top">
      {NAV_ITEMS.map((item) => (
        <a
          key={item.id}
          className={activeNav === item.id ? 'on' : ''}
          onClick={() => onSelectNav(item.id)}
        >
          <Icon name={item.icon} size={14} />
          {item.id}
        </a>
      ))}
    </div>
  );
};
