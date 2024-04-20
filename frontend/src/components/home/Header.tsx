import { Row } from "react-bootstrap";
import aurify from '../../assets/aurify.svg'

function Header(){
    return (
        <Row className="text-center justify-content-center align-content-center mt-2">
            <h1 className="text-green-400 col-4">
                Aurify
            </h1>
            {/* <img className="col-4" src={aurify} alt="App logo" /> */}
        </Row>
        
    )
}

export default Header;