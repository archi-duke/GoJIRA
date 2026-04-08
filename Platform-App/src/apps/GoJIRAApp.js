import React, { useState, useCallback, useEffect } from 'react';
import ProjectMap from './gojira/ProjectMap';
import IssueList from './gojira/IssueList';
import RelationMap from './gojira/RelationMap';
import SATAM from './gojira/SATAM';
import DocEditor from '../components/DocEditor';

export default function GoJIRAApp({ user, authority, resetKey }) {
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');

  // 타이틀바 Project 클릭 시 map으로 리셋
  useEffect(() => {
    if (resetKey > 0) {
      setPage('map');
      setSelectedProject(null);
      sessionStorage.setItem('gojira_page', 'map');
      sessionStorage.removeItem('gojira_project');
    }
  }, [resetKey]); // eslint-disable-line
  const [selectedProject, setSelectedProject] = useState(() => {
    try { return JSON.parse(sessionStorage.getItem('gojira_project')); }
    catch { return null; }
  });

  const navigate = useCallback((p, project) => {
    setPage(p);
    setSelectedProject(project ?? null);
    sessionStorage.setItem('gojira_page', p);
    if (project) sessionStorage.setItem('gojira_project', JSON.stringify(project));
    else sessionStorage.removeItem('gojira_project');
  }, []);

  const onNavigate = useCallback((target) => {
    navigate(target, selectedProject);
  }, [navigate, selectedProject]);

  if (page === 'docs' && selectedProject) {
    return <DocEditor project={selectedProject} user={user} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} />;
  }
  if (page === 'issues' && selectedProject) {
    return <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  }
  if (page === 'relationMap' && selectedProject) {
    return <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  }
  if (page === 'satam' && selectedProject) {
    return <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  }

  return (
    <ProjectMap
      onProjectClick={(p) => navigate('issues', p)}
      onRelationMap={(p) => navigate('relationMap', p)}
      onSATAM={(p) => navigate('satam', p)}
      user={user}
    />
  );
}
