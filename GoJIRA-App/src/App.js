import { useState, useCallback } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';

function App() {
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
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

  if (page === 'issues' && selectedProject) {
    return (
      <IssueList
        project={selectedProject}
        onBack={() => navigate('map')}
        currentPage={page}
        onNavigate={onNavigate}
      />
    );
  }

  if (page === 'relationMap' && selectedProject) {
    return (
      <RelationMap
        project={selectedProject}
        onBack={() => navigate('map')}
        currentPage={page}
        onNavigate={onNavigate}
      />
    );
  }

  if (page === 'satam' && selectedProject) {
    return (
      <SATAM
        project={selectedProject}
        onBack={() => navigate('map')}
        currentPage={page}
        onNavigate={onNavigate}
      />
    );
  }

  return (
    <ProjectMap
      onProjectClick={(project) => navigate('issues', project)}
      onRelationMap={(project) => navigate('relationMap', project)}
      onSATAM={(project) => navigate('satam', project)}
    />
  );
}

export default App;
