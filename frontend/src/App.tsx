import { useEffect, useState } from 'react'
import { Container } from 'react-bootstrap'
import { Outlet } from 'react-router-dom'

function App() {
  const [user, setUser] = useState({});
  

  useEffect(()=>{
    console.log(user);
  }, [user])

  return (
    <Container fluid>
      <h1 
        style={{userSelect: 'none'}}
        className='text-8xl text-green-500 mt-4 text-center kaushan-script-regular'>
          Aurify
      </h1>
      <main 
        id='current-route'
        className="d-flex mt-5 container-fluid justify-content-center"
      >
        <Outlet context={setUser}/>
      </main>
    </Container>
  )
}

export default App
