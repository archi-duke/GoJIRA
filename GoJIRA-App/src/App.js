import { useState } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';

function App() {
  const [page, setPage] = useState('map');
  const [selectedProject, setSelectedProject] = useState(null);

  if (page === 'issues' && selectedProject) {
    return (
      <IssueList
        project={selectedProject}
        onBack={() => setPage('map')}
      />
    );
  }

  if (page === 'relationMap' && selectedProject) {
    return (
      <RelationMap
        project={selectedProject}
        onBack={() => setPage('map')}
      />
    );
  }

  return (
    <ProjectMap
      onProjectClick={(project) => {
        setSelectedProject(project);
        setPage('issues');
      }}
      onRelationMap={(project) => {
        setSelectedProject(project);
        setPage('relationMap');
      }}
    />
  );
}

export default App;
